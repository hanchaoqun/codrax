package hitraceconv

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type traceDBSyscallTestOptions struct {
	lifecycle       traceDBLifecycleIndex
	complete        bool
	mutateIntegrity func(*traceDBRunningIntegrity)
	controls        []traceDBSyncSpanCandidate
	controlsAfter   []traceDBSyncSpanCandidate
	addNonSync      bool
	stageOptions    *traceDBSyncSpanStageOptions
	authorityResult *TraceDBCoverage
}

func traceDBSyscallTestStatements(runningRows, syscallRows []string) []string {
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'app-one')",
		"INSERT INTO process VALUES (2, 200, 'app-two')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 101, 1, 'worker-one', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 202, 2, 'worker-two', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
	}
	statements = append(statements, runningRows...)
	statements = append(statements, "CREATE TABLE syscall (ts, dur, syscall_number, itid)")
	return append(statements, syscallRows...)
}

func exportTraceDBSyscallTestFixture(t *testing.T, statements []string, options traceDBSyscallTestOptions) (
	TraceDBCoverage, traceDBSyncSpanReport, string,
) {
	t.Helper()
	ctx := context.Background()
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	identities, _, err := tdb.loadThreadIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	intervals, integrity, _, err := tdb.loadRunningIntervals(ctx, identities)
	if err != nil {
		t.Fatal(err)
	}
	if options.mutateIntegrity != nil {
		options.mutateIntegrity(&integrity)
	}
	authority := newTraceDBSchedulerAuthority(identities, traceDBLifecycleCollection{
		Lifecycle:        options.lifecycle,
		CreationComplete: options.complete,
		TerminalComplete: options.complete,
		ActivityComplete: options.complete,
	})
	running := newTraceDBSchedulerRunningIndex(authority, intervals, integrity, nil)
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	var syncSpans *traceDBSyncSpanAuthority
	if options.stageOptions == nil {
		syncSpans = newTraceDBTestSyncSpanAuthority(t)
	} else {
		stageOptions := *options.stageOptions
		if stageOptions.TempRoot == "" {
			stageOptions.TempRoot = t.TempDir()
		}
		syncSpans, err = newTraceDBSyncSpanAuthorityWithOptions(ctx,
			filepath.Join(t.TempDir(), "out.systrace"), stageOptions)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := syncSpans.cleanup(); err != nil {
				t.Errorf("cleanup syscall staged authority: %v", err)
			}
		})
	}
	for _, control := range options.controls {
		if err := syncSpans.submit(ctx, control); err != nil {
			t.Fatalf("submit control: %v", err)
		}
	}
	coverage, err := exportTraceDBSyscall(ctx, tdb, sink, authority, running, syncSpans)
	if err != nil {
		t.Fatalf("export syscall fixture: %v coverage=%+v", err, coverage)
	}
	for _, control := range options.controlsAfter {
		if err := syncSpans.submit(ctx, control); err != nil {
			t.Fatalf("submit post-export control: %v", err)
		}
	}
	if options.addNonSync {
		if err := addTraceDBAsyncSpanRows(sink, 5000, 5100, "async-control", 101, 100, 1, 2, "async-control", "cookie"); err != nil {
			t.Fatalf("add async control: %v", err)
		}
		if err := addTraceDBInstantRow(sink, 5200, "instant-control", 101, 100, 1,
			"tracing_mark_write: I|100|instant-control"); err != nil {
			t.Fatalf("add instant control: %v", err)
		}
		if err := addTraceDBInstantRow(sink, 5300, "counter-control", 101, 100, 1,
			"tracing_mark_write: C|100|counter-control|1"); err != nil {
			t.Fatalf("add counter control: %v", err)
		}
	}
	report, authorityCoverage, err := syncSpans.finalize(ctx, sink)
	if err != nil {
		t.Fatalf("finalize syscall fixture: %v coverage=%+v", err, authorityCoverage)
	}
	if options.authorityResult != nil {
		*options.authorityResult = authorityCoverage
	}
	rows := append([]renderedRow(nil), sink.rows...)
	sortRenderedRows(rows)
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, row.line)
	}
	return coverage, report, strings.Join(lines, "\n")
}

func traceDBSyscallControl(producer traceDBSyncSpanProducer, stableID, tid, tgid int64, name string) traceDBSyncSpanCandidate {
	candidate := traceDBTestSyncSpanCandidate(producer, stableID, tid, tgid, 4000, 4100, name)
	candidate.CanonicalITID = stableID
	return candidate
}

func TestTraceDBSyscallTypedEndpointCPUAndZeroPoint(t *testing.T) {
	statements := traceDBSyscallTestStatements([]string{
		"INSERT INTO thread_state VALUES (1, 0, 1500, 0, 'Running')",
		"INSERT INTO thread_state VALUES (1, 1500, 2500, 4095, 'Running')",
	}, []string{
		"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1200, 300, 9, 1)",
		"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (2, 0, 0, 10, 1)",
	})
	coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{complete: true})
	if coverage.RowsRead != 2 || report.SubmittedSpans != 2 || report.EmittedEndpoints != 4 {
		t.Fatalf("valid syscall rows not emitted: coverage=%+v report=%+v body=%q", coverage, report, body)
	}
	for _, want := range []string{"[000]", "[4095]", "B|100|sys_9", "B|100|sys_10"} {
		if !strings.Contains(body, want) {
			t.Fatalf("typed syscall output missing %q:\n%s", want, body)
		}
	}
	startMapped, endMapped := false, false
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "B|100|sys_9") && strings.Contains(line, "[000]") {
			startMapped = true
		}
		if strings.Contains(line, "E|100|") && strings.Contains(line, "[4095]") {
			endMapped = true
		}
	}
	if !startMapped || !endMapped {
		t.Fatalf("typed syscall endpoint CPUs are not bound to start/end: start=%t end=%t\n%s", startMapped, endMapped, body)
	}
	if !strings.Contains(coverage.FieldSources["cpu"], "typed Running") ||
		!strings.Contains(coverage.FieldSources["internal_identity"], "signed-int32") ||
		strings.Contains(coverage.FieldSources["source_admission"], "remain open") {
		t.Fatalf("syscall closure provenance missing: %+v", coverage.FieldSources)
	}
}

func TestTraceDBSyscallStrictScalarsPoisonWithoutRescue(t *testing.T) {
	fields := []string{"ts", "dur", "syscall_number", "itid"}
	invalid := []struct {
		name  string
		value string
	}{
		{name: "null", value: "NULL"},
		{name: "text", value: "CAST('7' AS TEXT)"},
		{name: "real", value: "CAST(7 AS REAL)"},
		{name: "blob", value: "X'07'"},
	}
	for _, field := range fields {
		for _, bad := range invalid {
			t.Run(field+"/"+bad.name, func(t *testing.T) {
				values := map[string]string{"ts": "1000", "dur": "100", "syscall_number": "9", "itid": "1"}
				values[field] = bad.value
				statements := traceDBSyscallTestStatements([]string{
					"INSERT INTO thread_state VALUES (1, 0, 5000, 1, 'Running')",
					"INSERT INTO thread_state VALUES (2, 0, 5000, 2, 'Running')",
				}, []string{fmt.Sprintf(
					"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, %s, %s, %s, %s)",
					values["ts"], values["dur"], values["syscall_number"], values["itid"])})
				controls := []traceDBSyncSpanCandidate{
					traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 11, 101, 100, "same-lane-control"),
					traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 22, 202, 200, "other-lane-control"),
				}
				coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
					complete: true, controls: controls,
				})
				if coverage.Skipped == "" || strings.Contains(body, "sys_None") {
					t.Fatalf("malformed scalar was repaired: coverage=%+v body=%q", coverage, body)
				}
				if field == "itid" {
					if !report.GlobalPoisoned || report.EmittedEndpoints != 0 || body != "" {
						t.Fatalf("unlocalizable syscall did not globally fail close: report=%+v body=%q", report, body)
					}
					return
				}
				if report.GlobalPoisoned || report.PoisonedLanes != 1 || report.EmittedEndpoints != 2 ||
					strings.Contains(body, "same-lane-control") || !strings.Contains(body, "other-lane-control") {
					t.Fatalf("exact malformed syscall lane was rescued or over-poisoned: report=%+v body=%q", report, body)
				}
			})
		}
	}
}

func TestTraceDBSyscallLargeInvalidCellUsesBoundedTransport(t *testing.T) {
	running := []string{
		"INSERT INTO thread_state VALUES (1, 0, 5000, 1, 'Running')",
		"INSERT INTO thread_state VALUES (2, 0, 5000, 2, 'Running')",
	}
	same := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 11, 101, 100, "same-lane")
	other := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 22, 202, 200, "other-lane")
	t.Run("large blob with exact identity poisons only its lane", func(t *testing.T) {
		statements := traceDBSyscallTestStatements(running, []string{
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, zeroblob(1048576), 100, 9, 1)",
		})
		coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			complete: true, controls: []traceDBSyncSpanCandidate{same, other},
		})
		if report.GlobalPoisoned || report.PoisonedLanes != 1 || report.EmittedEndpoints != 2 ||
			strings.Contains(body, "same-lane") || !strings.Contains(body, "other-lane") ||
			!strings.Contains(coverage.Skipped, "invalid_timestamp=1") {
			t.Fatalf("large invalid scalar escaped bounded exact poison: coverage=%+v report=%+v body=%q", coverage, report, body)
		}
	})
	t.Run("large blob identity globally fails closed", func(t *testing.T) {
		statements := traceDBSyscallTestStatements(running, []string{
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 100, 9, zeroblob(1048576))",
		})
		coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			complete: true, controls: []traceDBSyncSpanCandidate{same, other},
		})
		if !report.GlobalPoisoned || report.EmittedEndpoints != 0 || body != "" ||
			!strings.Contains(coverage.Skipped, "invalid_emitter_itid=1") {
			t.Fatalf("large invalid identity escaped bounded global poison: coverage=%+v report=%+v body=%q", coverage, report, body)
		}
	})
}

func TestTraceDBSyscallLargeInvalidCellProductionPathIsBounded(t *testing.T) {
	for _, test := range []struct {
		name       string
		row        string
		wantReason string
	}{
		{name: "timestamp blob", row: "INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, zeroblob(1048576), 100, 9, 1)", wantReason: "invalid_timestamp=1"},
		{name: "identity blob", row: "INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 100, 9, zeroblob(1048576))", wantReason: "invalid_emitter_itid=1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			statements := traceDBSyscallTestStatements([]string{
				"INSERT INTO thread_state VALUES (1, 0, 5000, 1, 'Running')",
			}, []string{test.row})
			path := createTraceDBFixture(t, statements)
			outPath := filepath.Join(t.TempDir(), "bounded-production.systrace")
			result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
			if err != nil {
				t.Fatalf("production export large malformed syscall: %v coverage=%+v", err, result.Coverage)
			}
			syscallCoverage := requireTraceDBCoverage(t, result.Coverage, "slice", "syscall")
			lifecycleCoverage := requireTraceDBCoverage(t, result.Coverage, "resolver.lifecycle.activity", "syscall")
			if syscallCoverage.RowsRead != 1 || syscallCoverage.RowsEmitted != 0 ||
				!strings.Contains(syscallCoverage.Skipped, test.wantReason) ||
				!strings.Contains(syscallCoverage.Skipped, "sync_family_source_fail_closed=unlocalizable_syscall_candidate") && test.name == "identity blob" ||
				lifecycleCoverage.RowsRead != 1 ||
				!strings.Contains(lifecycleCoverage.FieldSources["physical_rows"], "bounded INTEGER transport") {
				t.Fatalf("production bounded syscall path drifted: syscall=%+v lifecycle=%+v", syscallCoverage, lifecycleCoverage)
			}
		})
	}
}

func TestTraceDBSyscallNegativeAndOverflowRowsPoisonExactLane(t *testing.T) {
	tests := []struct {
		name       string
		ts         int64
		dur        int64
		wantReason string
	}{
		{name: "negative timestamp", ts: -1, dur: 1, wantReason: "invalid_timestamp=1"},
		{name: "negative duration", ts: 1, dur: -1, wantReason: "invalid_duration=1"},
		{name: "checked end overflow", ts: math.MaxInt64, dur: 1, wantReason: "timestamp_overflow=1"},
		{name: "MaxInt64 zero point does not overflow", ts: math.MaxInt64, dur: 0, wantReason: "unknown_start_cpu=1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statements := traceDBSyscallTestStatements([]string{
				"INSERT INTO thread_state VALUES (1, 0, 5000, 1, 'Running')",
				"INSERT INTO thread_state VALUES (2, 0, 5000, 2, 'Running')",
			}, []string{fmt.Sprintf(
				"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, %d, %d, 9, 1)", test.ts, test.dur)})
			controls := []traceDBSyncSpanCandidate{
				traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 11, 101, 100, "same-lane-control"),
				traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 22, 202, 200, "other-lane-control"),
			}
			coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{complete: true, controls: controls})
			if report.GlobalPoisoned || report.PoisonedLanes != 1 || report.EmittedEndpoints != 2 ||
				strings.Contains(body, "same-lane-control") || !strings.Contains(body, "other-lane-control") ||
				!strings.Contains(coverage.Skipped, test.wantReason) {
				t.Fatalf("invalid interval escaped exact-lane poison: coverage=%+v report=%+v body=%q", coverage, report, body)
			}
			if test.dur == 0 && strings.Contains(coverage.Skipped, "timestamp_overflow") {
				t.Fatalf("MaxInt64 zero point was falsely classified as overflow: %+v", coverage)
			}
		})
	}
}

func TestTraceDBSyscallSignedIdentityProfile(t *testing.T) {
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 300, 'high-process')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (4294967294, 303, 1, 'high-thread', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (4294967294, 0, 5000, 7, 'Running')",
		"CREATE TABLE syscall (ts, dur, syscall_number, itid)",
		"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (-1, 1000, 100, 11, -2)",
	}
	coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{complete: true})
	if coverage.RowsRead != 1 || report.SubmittedSpans != 1 || report.EmittedEndpoints != 2 ||
		!strings.Contains(body, "high-thread") || !strings.Contains(body, "B|300|sys_11") {
		t.Fatalf("signed high-half syscall identity lost: coverage=%+v report=%+v body=%q", coverage, report, body)
	}

	for _, raw := range []string{"-1", "2147483648"} {
		t.Run("reject-"+raw, func(t *testing.T) {
			bad := append([]string(nil), statements[:len(statements)-1]...)
			bad = append(bad, fmt.Sprintf(
				"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 100, 11, %s)", raw))
			control := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 33, 303, 300, "global-control")
			_, rejected, rejectedBody := exportTraceDBSyscallTestFixture(t, bad, traceDBSyscallTestOptions{
				complete: true, controls: []traceDBSyncSpanCandidate{control},
			})
			if !rejected.GlobalPoisoned || rejected.EmittedEndpoints != 0 || rejectedBody != "" {
				t.Fatalf("invalid signed profile encoding escaped: raw=%s report=%+v body=%q", raw, rejected, rejectedBody)
			}
		})
	}
}

func TestTraceDBSyscallNumberSignedProjection(t *testing.T) {
	baseRunning := []string{"INSERT INTO thread_state VALUES (1, 0, 5000, 7, 'Running')"}
	valid := traceDBSyscallTestStatements(baseRunning, []string{
		"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 100, -2, 1)",
	})
	coverage, report, body := exportTraceDBSyscallTestFixture(t, valid, traceDBSyscallTestOptions{complete: true})
	if coverage.RowsRead != 1 || report.EmittedEndpoints != 2 ||
		!strings.Contains(body, "sys_4294967294") || strings.Contains(body, "sys_-2") {
		t.Fatalf("signed syscall number projection drifted: coverage=%+v report=%+v body=%q", coverage, report, body)
	}
	for _, raw := range []string{"-1", "2147483648"} {
		t.Run("reject-"+raw, func(t *testing.T) {
			statements := traceDBSyscallTestStatements(baseRunning, []string{fmt.Sprintf(
				"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 100, %s, 1)", raw)})
			control := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 11, 101, 100, "same-lane-control")
			coverage, rejected, rejectedBody := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
				complete: true, controls: []traceDBSyncSpanCandidate{control},
			})
			if rejected.GlobalPoisoned || rejected.PoisonedLanes != 1 || rejected.EmittedEndpoints != 0 ||
				rejectedBody != "" || !strings.Contains(coverage.Skipped, "invalid_syscall_number=1") {
				t.Fatalf("invalid syscall number escaped exact-lane poison: raw=%s coverage=%+v report=%+v body=%q",
					raw, coverage, rejected, rejectedBody)
			}
		})
	}
}

func TestTraceDBSyscallRejectedRowAntiRescueIsOrderIndependent(t *testing.T) {
	for _, badRowID := range []int{-1, 2} {
		t.Run(fmt.Sprintf("bad-rowid-%d", badRowID), func(t *testing.T) {
			rows := []string{
				"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (0, 1000, 100, 10, 1)",
				fmt.Sprintf("INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (%d, 1200, 100, NULL, 1)", badRowID),
				"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1400, 100, 11, 1)",
			}
			statements := traceDBSyscallTestStatements([]string{
				"INSERT INTO thread_state VALUES (1, 0, 5000, 1, 'Running')",
				"INSERT INTO thread_state VALUES (2, 0, 5000, 2, 'Running')",
			}, rows)
			before := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 21, 101, 100, "before-same-lane")
			after := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 22, 101, 100, "after-same-lane")
			other := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 23, 202, 200, "other-lane")
			coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
				complete: true,
				controls: []traceDBSyncSpanCandidate{before}, controlsAfter: []traceDBSyncSpanCandidate{after, other},
			})
			if coverage.RowsRead != 3 || report.SubmittedSpans != 5 || report.SuppressedSpans != 4 ||
				report.EmittedEndpoints != 2 || report.PoisonedLanes != 1 ||
				strings.Contains(body, "sys_10") || strings.Contains(body, "sys_11") ||
				strings.Contains(body, "same-lane") || !strings.Contains(body, "other-lane") {
				t.Fatalf("physical row order rescued a poisoned syscall lane: coverage=%+v report=%+v body=%q",
					coverage, report, body)
			}
		})
	}
}

func TestTraceDBSyscallUnplaceableRowGloballyFailsClosedSyncOnly(t *testing.T) {
	statements := traceDBSyscallTestStatements([]string{
		"INSERT INTO thread_state VALUES (1, 0, 5000, 1, 'Running')",
		"INSERT INTO thread_state VALUES (2, 0, 5000, 2, 'Running')",
	}, []string{
		"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (0, 1000, 100, 10, 1)",
		"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1200, 100, 11, NULL)",
		"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (2, 1400, 100, 12, 2)",
	})
	before := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 21, 101, 100, "before-global")
	after := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 22, 202, 200, "after-global")
	coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
		complete: true, controls: []traceDBSyncSpanCandidate{before}, controlsAfter: []traceDBSyncSpanCandidate{after}, addNonSync: true,
	})
	if coverage.RowsRead != 3 || !report.GlobalPoisoned || report.SubmittedSpans != 4 ||
		report.SuppressedSpans != 4 || report.EmittedEndpoints != 0 ||
		strings.Contains(body, "sys_10") || strings.Contains(body, "sys_12") || strings.Contains(body, "global") {
		t.Fatalf("unplaceable syscall did not suppress every pre/post B/E candidate: coverage=%+v report=%+v body=%q",
			coverage, report, body)
	}
	for _, want := range []string{"S|100|async-control|cookie", "F|100|async-control|cookie", "I|100|instant-control", "C|100|counter-control|1"} {
		if !strings.Contains(body, want) {
			t.Fatalf("global sync poison escaped its B/E scope and removed %q:\n%s", want, body)
		}
	}
}

func TestTraceDBSyscallSchemaAndStableIdentityFailClosed(t *testing.T) {
	base := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'app-one')",
		"INSERT INTO process VALUES (2, 200, 'app-two')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 101, 1, 'worker-one', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 202, 2, 'worker-two', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (1, 0, 5000, 1, 'Running')",
		"INSERT INTO thread_state VALUES (2, 0, 5000, 2, 'Running')",
	}
	same := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 21, 101, 100, "same-lane")
	other := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 22, 202, 200, "other-lane")

	t.Run("empty missing-column table is a local no-op", func(t *testing.T) {
		statements := append(append([]string(nil), base...), "CREATE TABLE syscall (ts, syscall_number, itid)")
		coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			complete: true, controls: []traceDBSyncSpanCandidate{same, other},
		})
		if coverage.RowsRead != 0 || report.GlobalPoisoned || report.PoisonedLanes != 0 ||
			report.EmittedEndpoints != 4 || !strings.Contains(body, "same-lane") || !strings.Contains(body, "other-lane") {
			t.Fatalf("empty incomplete schema dragged sync authority: coverage=%+v report=%+v body=%q", coverage, report, body)
		}
	})

	t.Run("nonempty missing field poisons exact lanes", func(t *testing.T) {
		statements := append(append([]string(nil), base...),
			"CREATE TABLE syscall (ts, syscall_number, itid)",
			"INSERT INTO syscall VALUES (1000, 9, 1)")
		coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			complete: true, controls: []traceDBSyncSpanCandidate{same, other},
		})
		if coverage.RowsRead != 1 || report.GlobalPoisoned || report.PoisonedLanes != 1 ||
			report.EmittedEndpoints != 2 || strings.Contains(body, "same-lane") || !strings.Contains(body, "other-lane") {
			t.Fatalf("localizable incomplete schema escaped lane poison: coverage=%+v report=%+v body=%q", coverage, report, body)
		}
	})

	t.Run("nonempty missing identity globally fails closed", func(t *testing.T) {
		statements := append(append([]string(nil), base...),
			"CREATE TABLE syscall (ts, dur, syscall_number)",
			"INSERT INTO syscall VALUES (1000, 100, 9)")
		coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			complete: true, controls: []traceDBSyncSpanCandidate{same, other},
		})
		if coverage.RowsRead != 1 || !report.GlobalPoisoned || report.EmittedEndpoints != 0 || body != "" {
			t.Fatalf("unlocalizable incomplete schema did not globally fail close: coverage=%+v report=%+v body=%q", coverage, report, body)
		}
	})

	t.Run("all hidden rowid aliases shadowed poison exact lane", func(t *testing.T) {
		statements := append(append([]string(nil), base...),
			"CREATE TABLE syscall (rowid TEXT, _rowid_ TEXT, oid TEXT, ts, dur, syscall_number, itid)",
			"INSERT INTO syscall VALUES ('r', 'u', 'o', 1000, 100, 9, 1)")
		coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			complete: true, controls: []traceDBSyncSpanCandidate{same, other},
		})
		if coverage.RowsRead != 1 || report.GlobalPoisoned || report.PoisonedLanes != 1 ||
			report.EmittedEndpoints != 2 || strings.Contains(body, "same-lane") || !strings.Contains(body, "other-lane") ||
			!strings.Contains(coverage.Skipped, "stable_row_identity_unavailable=1") {
			t.Fatalf("shadowed hidden identity escaped exact-lane poison: coverage=%+v report=%+v body=%q", coverage, report, body)
		}
	})
}

func TestTraceDBSyscallIdentityOwnerAndCaptureDomain(t *testing.T) {
	t.Run("idle identity is unlocalizable", func(t *testing.T) {
		statements := traceDBSyscallTestStatements(nil, []string{
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 0, 9, 0)",
		})
		control := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 21, 202, 200, "global-control")
		_, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			complete: true, controls: []traceDBSyncSpanCandidate{control},
		})
		if !report.GlobalPoisoned || report.EmittedEndpoints != 0 || body != "" {
			t.Fatalf("idle syscall identity escaped global fail-close: report=%+v body=%q", report, body)
		}
	})

	t.Run("PID zero rejects but preserves exact physical lane scope", func(t *testing.T) {
		statements := []string{
			"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid, pid, name)",
			"INSERT INTO process VALUES (1, 0, 'pid-zero')", "INSERT INTO process VALUES (2, 200, 'valid')",
			"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
			"INSERT INTO thread VALUES (1, 101, 1, 'pid-zero-thread', 0, 0, 1)",
			"INSERT INTO thread VALUES (2, 202, 2, 'valid-thread', 0, 0, 1)",
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
			"INSERT INTO thread_state VALUES (1, 0, 5000, 1, 'Running')",
			"INSERT INTO thread_state VALUES (2, 0, 5000, 2, 'Running')",
			"CREATE TABLE syscall (ts, dur, syscall_number, itid)",
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 100, 9, 1)",
		}
		same := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 21, 101, 100, "same-lane")
		other := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 22, 202, 200, "other-lane")
		coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			complete: true, controls: []traceDBSyncSpanCandidate{same, other},
		})
		if report.GlobalPoisoned || report.PoisonedLanes != 1 || report.EmittedEndpoints != 2 ||
			strings.Contains(body, "same-lane") || !strings.Contains(body, "other-lane") ||
			!strings.Contains(coverage.Skipped, "unresolved_emitter_identity=1") {
			t.Fatalf("PID0 syscall poison scope drifted: coverage=%+v report=%+v body=%q", coverage, report, body)
		}
	})

	t.Run("ambiguous ITID globally fails closed", func(t *testing.T) {
		statements := []string{
			"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid, pid, name)", "INSERT INTO process VALUES (1, 100, 'app')",
			"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
			"INSERT INTO thread VALUES (1, 101, 1, 'first', 0, 0, 1)",
			"INSERT INTO thread VALUES (1, 102, 1, 'second', 0, 0, 1)",
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
			"CREATE TABLE syscall (ts, dur, syscall_number, itid)",
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 100, 9, 1)",
		}
		control := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 21, 303, 300, "global-control")
		_, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			complete: true, controls: []traceDBSyncSpanCandidate{control},
		})
		if !report.GlobalPoisoned || report.EmittedEndpoints != 0 || body != "" {
			t.Fatalf("ambiguous syscall ITID escaped global fail-close: report=%+v body=%q", report, body)
		}
	})

	t.Run("non-positive public TID globally fails closed", func(t *testing.T) {
		statements := []string{
			"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid, pid, name)", "INSERT INTO process VALUES (1, 100, 'app')",
			"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
			"INSERT INTO thread VALUES (1, 0, 1, 'tid-zero', 0, 0, 1)",
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
			"CREATE TABLE syscall (ts, dur, syscall_number, itid)",
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 100, 9, 1)",
		}
		control := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 21, 202, 200, "global-control")
		coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			complete: true, controls: []traceDBSyncSpanCandidate{control},
		})
		if !report.GlobalPoisoned || report.EmittedEndpoints != 0 || body != "" ||
			!strings.Contains(coverage.Skipped, "unresolved_emitter_identity=1") {
			t.Fatalf("TID0 syscall escaped global fail-close: coverage=%+v report=%+v body=%q", coverage, report, body)
		}
	})

	t.Run("capture-start predecessor poisons exact lane", func(t *testing.T) {
		statements := []string{
			"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (1000)",
			"CREATE TABLE process (ipid, pid, name)", "INSERT INTO process VALUES (1, 100, 'app')",
			"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
			"INSERT INTO thread VALUES (1, 101, 1, 'worker', 0, 0, 1)",
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
			"INSERT INTO thread_state VALUES (1, 900, 500, 1, 'Running')",
			"CREATE TABLE syscall (ts, dur, syscall_number, itid)",
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 999, 0, 9, 1)",
		}
		control := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 21, 101, 100, "same-lane")
		coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			complete: true, controls: []traceDBSyncSpanCandidate{control},
		})
		if report.GlobalPoisoned || report.PoisonedLanes != 1 || report.EmittedEndpoints != 0 || body != "" ||
			!strings.Contains(coverage.Skipped, "before_capture_start=1") {
			t.Fatalf("pre-capture syscall escaped: coverage=%+v report=%+v body=%q", coverage, report, body)
		}
	})

	t.Run("canonical IPID zero with positive PID remains valid", func(t *testing.T) {
		statements := []string{
			"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid, pid, name)", "INSERT INTO process VALUES (0, 100, 'ipid-zero')",
			"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
			"INSERT INTO thread VALUES (1, 101, 0, 'worker', 0, 0, 1)",
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
			"INSERT INTO thread_state VALUES (1, 0, 5000, 1, 'Running')",
			"CREATE TABLE syscall (ts, dur, syscall_number, itid)",
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 100, 9, 1)",
		}
		coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{complete: true})
		if coverage.RowsRead != 1 || report.EmittedEndpoints != 2 || !strings.Contains(body, "B|100|sys_9") {
			t.Fatalf("valid IPID0 owner was rejected: coverage=%+v report=%+v body=%q", coverage, report, body)
		}
	})
}

func TestTraceDBSyscallRenameDoesNotChangeAdmission(t *testing.T) {
	run := func(t *testing.T, name string) (traceDBSyncSpanReport, string) {
		t.Helper()
		statements := []string{
			"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid, pid, name)", "INSERT INTO process VALUES (1, 100, 'app')",
			"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
			fmt.Sprintf("INSERT INTO thread VALUES (1, 101, 1, '%s', 0, 0, 1)", name),
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
			"INSERT INTO thread_state VALUES (1, 0, 5000, 1, 'Running')",
			"CREATE TABLE syscall (ts, dur, syscall_number, itid)",
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 100, 9, 1)",
		}
		_, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{complete: true})
		return report, body
	}
	leftReport, leftBody := run(t, "before-rename")
	rightReport, rightBody := run(t, "after-rename")
	if leftReport.SubmittedSpans != rightReport.SubmittedSpans || leftReport.EmittedEndpoints != rightReport.EmittedEndpoints ||
		leftReport.SuppressedSpans != rightReport.SuppressedSpans ||
		!strings.Contains(leftBody, "before-rename") || !strings.Contains(rightBody, "after-rename") ||
		!strings.Contains(leftBody, "B|100|sys_9") || !strings.Contains(rightBody, "B|100|sys_9") {
		t.Fatalf("thread rename changed syscall admission: left=%+v %q right=%+v %q",
			leftReport, leftBody, rightReport, rightBody)
	}
}

func TestTraceDBSyscallLifecycleAndRunningWitnessRejectClosedEnd(t *testing.T) {
	statements := traceDBSyscallTestStatements([]string{
		"INSERT INTO thread_state VALUES (1, 0, 5000, 3, 'Running')",
		"INSERT INTO thread_state VALUES (2, 0, 5000, 4, 'Running')",
	}, []string{
		"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 1000, 9, 1)",
	})
	cut := traceDBLifecycleIndex{
		ByTID: map[int64]traceDBLifecycleLane{101: {Cuts: []traceDBLifecycleBoundary{{TS: 2000, NewITID: 2, NewIPID: 2}}}},
		ByPID: map[int64]traceDBLifecycleLane{100: {Cuts: []traceDBLifecycleBoundary{{TS: 2000, NewITID: 2, NewIPID: 2}}}},
	}
	control := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 22, 202, 200, "other-lane-control")
	coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
		lifecycle: cut, complete: true, controls: []traceDBSyncSpanCandidate{control},
	})
	if report.GlobalPoisoned || report.PoisonedLanes != 1 || report.EmittedEndpoints != 2 ||
		!strings.Contains(body, "other-lane-control") || !strings.Contains(coverage.Skipped, "lifecycle_rejected") {
		t.Fatalf("closed-end generation cut escaped: coverage=%+v report=%+v body=%q", coverage, report, body)
	}

	missingEnd := traceDBSyscallTestStatements([]string{
		"INSERT INTO thread_state VALUES (1, 0, 1500, 3, 'Running')",
		"INSERT INTO thread_state VALUES (2, 0, 5000, 4, 'Running')",
	}, []string{
		"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 1000, 9, 1)",
	})
	coverage, report, body = exportTraceDBSyscallTestFixture(t, missingEnd, traceDBSyscallTestOptions{
		complete: true, controls: []traceDBSyncSpanCandidate{control},
	})
	if report.GlobalPoisoned || report.PoisonedLanes != 1 || report.EmittedEndpoints != 2 ||
		!strings.Contains(body, "other-lane-control") || !strings.Contains(coverage.Skipped, "unknown_end_cpu") {
		t.Fatalf("unknown exact-end CPU escaped: coverage=%+v report=%+v body=%q", coverage, report, body)
	}
}

func TestTraceDBSyscallLifecycleBoundaryMatrix(t *testing.T) {
	cut := func(ts, newITID, newIPID int64, threadCut, processCut bool) traceDBLifecycleIndex {
		out := traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{}, ByPID: map[int64]traceDBLifecycleLane{}}
		boundary := traceDBLifecycleBoundary{TS: ts, NewITID: newITID, NewIPID: newIPID}
		if threadCut {
			out.ByTID[101] = traceDBLifecycleLane{Cuts: []traceDBLifecycleBoundary{boundary}}
		}
		if processCut {
			out.ByPID[100] = traceDBLifecycleLane{Cuts: []traceDBLifecycleBoundary{boundary}}
		}
		return out
	}
	tests := []struct {
		name      string
		lifecycle traceDBLifecycleIndex
		wantEmit  bool
	}{
		{name: "clean", lifecycle: traceDBLifecycleIndex{}, wantEmit: true},
		{name: "thread cut at start", lifecycle: cut(1000, 2, 2, true, false)},
		{name: "process cut interior", lifecycle: cut(1500, 2, 2, false, true)},
		{name: "both cut at closed end", lifecycle: cut(2000, 2, 2, true, true)},
		{name: "same identity cut at closed end", lifecycle: cut(2000, 1, 1, true, true)},
		{name: "future cut", lifecycle: cut(5001, 2, 2, true, true), wantEmit: true},
		{name: "global lifecycle point", lifecycle: traceDBLifecycleIndex{GlobalPoison: []int64{1500}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statements := traceDBSyscallTestStatements([]string{
				"INSERT INTO thread_state VALUES (1, 0, 5000, 3, 'Running')",
				"INSERT INTO thread_state VALUES (2, 0, 5000, 4, 'Running')",
			}, []string{
				"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 1000, 9, 1)",
			})
			other := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 22, 202, 200, "other-lane")
			coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
				lifecycle: test.lifecycle, complete: true, controls: []traceDBSyncSpanCandidate{other},
			})
			if test.wantEmit {
				if report.PoisonedLanes != 0 || report.EmittedEndpoints != 4 ||
					!strings.Contains(body, "sys_9") || !strings.Contains(body, "other-lane") {
					t.Fatalf("valid lifecycle interval rejected: coverage=%+v report=%+v body=%q", coverage, report, body)
				}
				return
			}
			if report.GlobalPoisoned || report.PoisonedLanes != 1 || report.EmittedEndpoints != 2 ||
				strings.Contains(body, "sys_9") || !strings.Contains(body, "other-lane") ||
				!strings.Contains(coverage.Skipped, "lifecycle_rejected") {
				t.Fatalf("lifecycle boundary escaped exact-lane poison: coverage=%+v report=%+v body=%q", coverage, report, body)
			}
		})
	}

	t.Run("old generation zero point at cut is rejected", func(t *testing.T) {
		statements := traceDBSyscallTestStatements([]string{
			"INSERT INTO thread_state VALUES (1, 0, 5000, 3, 'Running')",
			"INSERT INTO thread_state VALUES (2, 0, 5000, 4, 'Running')",
		}, []string{
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 0, 9, 1)",
		})
		other := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 22, 202, 200, "other-lane")
		coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			lifecycle: cut(1000, 2, 2, true, true), complete: true, controls: []traceDBSyncSpanCandidate{other},
		})
		if report.GlobalPoisoned || report.PoisonedLanes != 1 || report.EmittedEndpoints != 2 ||
			strings.Contains(body, "sys_9") || !strings.Contains(body, "other-lane") ||
			!strings.Contains(coverage.Skipped, "lifecycle_rejected") {
			t.Fatalf("old zero point at generation cut escaped: coverage=%+v report=%+v body=%q", coverage, report, body)
		}
	})

	t.Run("new generation at cut start is admitted", func(t *testing.T) {
		statements := []string{
			"CREATE TABLE trace_range (start_ts)", "INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid, pid, name)",
			"INSERT INTO process VALUES (1, 100, 'old')", "INSERT INTO process VALUES (2, 100, 'new')",
			"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
			"INSERT INTO thread VALUES (1, 101, 1, 'old-thread', 0, 0, 1)",
			"INSERT INTO thread VALUES (2, 101, 2, 'new-thread', 0, 0, 1)",
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
			"INSERT INTO thread_state VALUES (2, 1000, 4000, 4, 'Running')",
			"CREATE TABLE syscall (ts, dur, syscall_number, itid)",
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 100, 9, 2)",
		}
		lifecycle := traceDBLifecycleIndex{
			ByTID: map[int64]traceDBLifecycleLane{101: {Cuts: []traceDBLifecycleBoundary{{TS: 1000, NewITID: 2, NewIPID: 2}}}},
			ByPID: map[int64]traceDBLifecycleLane{100: {Cuts: []traceDBLifecycleBoundary{{TS: 1000, NewITID: 2, NewIPID: 2}}}},
		}
		coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			lifecycle: lifecycle, complete: true,
		})
		if coverage.RowsRead != 1 || report.PoisonedLanes != 0 || report.EmittedEndpoints != 2 ||
			!strings.Contains(body, "new-thread") || !strings.Contains(body, "sys_9") {
			t.Fatalf("new generation at exact cut was rejected: coverage=%+v report=%+v body=%q", coverage, report, body)
		}
	})
}

func TestTraceDBSyscallRunningWitnessRejectionMatrix(t *testing.T) {
	tests := []struct {
		name            string
		runningRows     []string
		lifecycle       traceDBLifecycleIndex
		mutateIntegrity func(*traceDBRunningIntegrity)
		wantReason      string
	}{
		{name: "missing start", runningRows: []string{"INSERT INTO thread_state VALUES (1, 1001, 4999, 3, 'Running')"}, wantReason: "unknown_start_cpu=1"},
		{name: "missing end", runningRows: []string{"INSERT INTO thread_state VALUES (1, 0, 1500, 3, 'Running')"}, wantReason: "unknown_end_cpu=1"},
		{name: "source taint", runningRows: []string{"INSERT INTO thread_state VALUES (1, 0, 5000, 3, 'Running')"},
			mutateIntegrity: func(integrity *traceDBRunningIntegrity) { integrity.TaintedITIDs[1] = true },
			wantReason:      "tainted_running_cpu_witness=1"},
		{name: "lifecycle rejected Running lane", runningRows: []string{"INSERT INTO thread_state VALUES (1, 0, 5000, 3, 'Running')"},
			lifecycle: traceDBLifecycleIndex{
				ByTID: map[int64]traceDBLifecycleLane{101: {Cuts: []traceDBLifecycleBoundary{{TS: 3000, NewITID: 2, NewIPID: 2}}}},
				ByPID: map[int64]traceDBLifecycleLane{100: {Cuts: []traceDBLifecycleBoundary{{TS: 3000, NewITID: 2, NewIPID: 2}}}},
			},
			wantReason: "lifecycle_rejected_running_cpu_witness=1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runningRows := append([]string(nil), test.runningRows...)
			runningRows = append(runningRows, "INSERT INTO thread_state VALUES (2, 0, 5000, 4, 'Running')")
			statements := traceDBSyscallTestStatements(runningRows, []string{
				"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (1, 1000, 1000, 9, 1)",
			})
			other := traceDBSyscallControl(traceDBSyncSpanProducerCallstack, 22, 202, 200, "other-lane")
			coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
				lifecycle: test.lifecycle, complete: true, controls: []traceDBSyncSpanCandidate{other}, mutateIntegrity: test.mutateIntegrity,
			})
			if report.GlobalPoisoned || report.PoisonedLanes != 1 || report.EmittedEndpoints != 2 ||
				strings.Contains(body, "sys_9") || !strings.Contains(body, "other-lane") ||
				!strings.Contains(coverage.Skipped, test.wantReason) {
				t.Fatalf("Running witness rejection escaped: coverage=%+v report=%+v body=%q", coverage, report, body)
			}
		})
	}
}

func TestTraceDBSyscallMemorySQLiteAndTracequeryParity(t *testing.T) {
	statements := traceDBSyscallTestStatements([]string{
		"INSERT INTO thread_state VALUES (1, 0, 3000000, 3, 'Running')",
	}, []string{
		"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (-1, 1000000, 500000, 9, 1)",
	})
	type result struct {
		report            traceDBSyncSpanReport
		authorityCoverage TraceDBCoverage
		body              string
	}
	run := func(t *testing.T, options traceDBSyncSpanStageOptions) result {
		t.Helper()
		var authorityCoverage TraceDBCoverage
		_, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			complete: true, stageOptions: &options, authorityResult: &authorityCoverage,
		})
		return result{report: report, authorityCoverage: authorityCoverage, body: body + "\n"}
	}
	memory := run(t, traceDBSyncSpanStageOptions{})
	sqlite := run(t, traceDBSyncSpanStageOptions{ResidentBytes: 1})
	if !reflect.DeepEqual(memory.report, sqlite.report) || memory.body != sqlite.body ||
		memory.authorityCoverage.RowsEmitted != sqlite.authorityCoverage.RowsEmitted ||
		memory.authorityCoverage.Skipped != sqlite.authorityCoverage.Skipped ||
		memory.authorityCoverage.SpillChunks != 0 || sqlite.authorityCoverage.SpillChunks == 0 {
		t.Fatalf("syscall sync-stage backend parity drifted:\nmemory=%+v %q\nsqlite=%+v %q",
			memory, memory.body, sqlite, sqlite.body)
	}
	outPath := filepath.Join(t.TempDir(), "syscall-roundtrip.systrace")
	if err := os.WriteFile(outPath, []byte(memory.body), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("parse syscall round-trip: %v", err)
	}
	spans, caveats := tracequery.FindSpanWindows(index, tracequery.Query{PID: 101, SpanName: "sys_9"}, 4)
	if len(spans) != 1 || spans[0].Kind != "sync" || !nearFloat(spans[0].DurationMs, 0.5, 0.000001) {
		t.Fatalf("syscall tracequery round-trip mismatch: spans=%+v caveats=%v body=%q", spans, caveats, memory.body)
	}
	for _, caveat := range caveats {
		if strings.Contains(caveat, "pairing") || strings.Contains(caveat, "duplicate") {
			t.Fatalf("syscall round-trip gained pairing caveat: %v", caveats)
		}
	}
}

func TestTraceDBSyscallPoisonPermutationAndNearCapBackendParity(t *testing.T) {
	type result struct {
		coverage          TraceDBCoverage
		report            traceDBSyncSpanReport
		authorityCoverage TraceDBCoverage
		body              string
	}
	run := func(t *testing.T, badRowID int, options traceDBSyncSpanStageOptions) result {
		t.Helper()
		statements := traceDBSyscallTestStatements([]string{
			"INSERT INTO thread_state VALUES (1, 0, 3000000, 3, 'Running')",
			"INSERT INTO thread_state VALUES (2, 0, 3000000, 4, 'Running')",
		}, []string{
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (0, 1000000, 100000, 10, 1)",
			fmt.Sprintf("INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (%d, 1100000, 100000, NULL, 1)", badRowID),
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (2, 1200000, 100000, 11, 1)",
			"INSERT INTO syscall(rowid, ts, dur, syscall_number, itid) VALUES (4, 1400000, 100000, 20, 2)",
		})
		var authorityCoverage TraceDBCoverage
		coverage, report, body := exportTraceDBSyscallTestFixture(t, statements, traceDBSyscallTestOptions{
			complete: true, stageOptions: &options, authorityResult: &authorityCoverage,
		})
		return result{coverage: coverage, report: report, authorityCoverage: authorityCoverage, body: body}
	}
	backendOptions := []struct {
		name              string
		options           traceDBSyncSpanStageOptions
		wantExternal      bool
		wantResidentAbove int
	}{
		{name: "memory"},
		{name: "tiny-promotion", options: traceDBSyncSpanStageOptions{ResidentBytes: 500}, wantExternal: true, wantResidentAbove: 0},
		{name: "forced-sqlite", options: traceDBSyncSpanStageOptions{ResidentBytes: 1}, wantExternal: true, wantResidentAbove: -1},
	}
	var cleanReference *result
	for _, badRowID := range []int{-1, 1, 3, 5} {
		for _, backend := range backendOptions {
			t.Run(fmt.Sprintf("rowid-%d/%s", badRowID, backend.name), func(t *testing.T) {
				got := run(t, badRowID, backend.options)
				if got.coverage.RowsRead != 4 || got.report.SubmittedSpans != 3 || got.report.SuppressedSpans != 2 ||
					got.report.EmittedEndpoints != 2 || got.report.PoisonedLanes != 1 ||
					strings.Contains(got.body, "sys_10") || strings.Contains(got.body, "sys_11") ||
					!strings.Contains(got.body, "sys_20") || !strings.Contains(got.coverage.Skipped, "invalid_syscall_number=1") {
					t.Fatalf("poison/backend permutation drifted: %+v", got)
				}
				external := strings.HasPrefix(got.authorityCoverage.FieldSources["stage_backend"], "sqlite;")
				if external != backend.wantExternal ||
					backend.wantResidentAbove == 0 && got.authorityCoverage.PeakBuffered == 0 ||
					backend.wantResidentAbove < 0 && got.authorityCoverage.PeakBuffered != 0 {
					t.Fatalf("stage backend shape mismatch backend=%s coverage=%+v", backend.name, got.authorityCoverage)
				}
				if cleanReference == nil {
					copy := got
					cleanReference = &copy
					return
				}
				if !reflect.DeepEqual(got.report, cleanReference.report) || got.body != cleanReference.body ||
					got.coverage.RowsRead != cleanReference.coverage.RowsRead || got.coverage.Skipped != cleanReference.coverage.Skipped ||
					got.authorityCoverage.RowsEmitted != cleanReference.authorityCoverage.RowsEmitted ||
					got.authorityCoverage.Skipped != cleanReference.authorityCoverage.Skipped {
					t.Fatalf("backend/physical-order parity mismatch:\nreference=%+v\ngot=%+v", *cleanReference, got)
				}
			})
		}
	}

	var budgetReference *result
	for _, badRowID := range []int{-1, 1, 3, 5} {
		for _, backend := range backendOptions {
			t.Run(fmt.Sprintf("near-cap-rowid-%d/%s", badRowID, backend.name), func(t *testing.T) {
				options := backend.options
				options.MaxRecords = 3
				got := run(t, badRowID, options)
				if got.report.BudgetFailClosedReason != traceDBSyncSpanStageBudgetRecordCap ||
					got.report.SubmittedSpans != 3 || got.report.SuppressedSpans != 3 || got.report.EmittedEndpoints != 0 ||
					got.body != "" || !strings.Contains(got.authorityCoverage.Skipped, "sync_family_budget_fail_closed=record_cap") {
					t.Fatalf("near-cap fail-close drifted: %+v", got)
				}
				external := strings.HasPrefix(got.authorityCoverage.FieldSources["stage_backend"], "sqlite;")
				if external != backend.wantExternal ||
					backend.wantResidentAbove == 0 && got.authorityCoverage.PeakBuffered == 0 ||
					backend.wantResidentAbove < 0 && got.authorityCoverage.PeakBuffered != 0 {
					t.Fatalf("near-cap backend shape mismatch backend=%s coverage=%+v", backend.name, got.authorityCoverage)
				}
				if budgetReference == nil {
					copy := got
					budgetReference = &copy
					return
				}
				if !reflect.DeepEqual(got.report, budgetReference.report) || got.body != budgetReference.body ||
					got.coverage.Skipped != budgetReference.coverage.Skipped ||
					got.authorityCoverage.RowsEmitted != budgetReference.authorityCoverage.RowsEmitted ||
					got.authorityCoverage.Skipped != budgetReference.authorityCoverage.Skipped {
					t.Fatalf("near-cap backend/order parity mismatch:\nreference=%+v\ngot=%+v", *budgetReference, got)
				}
			})
		}
	}
}

func TestTraceDBSyscallSourceAdmissionStructurePinned(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(filepath.Dir(current), "streamerdb_export_extended.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	functions := map[string]*ast.FuncDecl{}
	for _, declaration := range file.Decls {
		if function, ok := declaration.(*ast.FuncDecl); ok {
			functions[function.Name.Name] = function
		}
	}
	exporter := functions["exportTraceDBSyscall"]
	prepare := functions["prepareTraceDBSyscallRow"]
	extended := functions["exportTraceDBExtendedFamilies"]
	if exporter == nil || prepare == nil || extended == nil {
		t.Fatal("syscall source-admission function closure is incomplete")
	}
	typeName := func(expression ast.Expr) string {
		switch typed := expression.(type) {
		case *ast.Ident:
			return typed.Name
		case *ast.StarExpr:
			if ident, ok := typed.X.(*ast.Ident); ok {
				return "*" + ident.Name
			}
		}
		return ""
	}
	isIdentExpr := func(expression ast.Expr, name string) bool {
		ident, ok := expression.(*ast.Ident)
		return ok && ident.Name == name
	}
	isSelectorExpr := func(expression ast.Expr, receiver, field string) bool {
		selector, ok := expression.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != field {
			return false
		}
		ident, ok := selector.X.(*ast.Ident)
		return ok && ident.Name == receiver
	}
	methodReceiver := func(call *ast.CallExpr, receiver, method string) bool {
		selector, ok := call.Fun.(*ast.SelectorExpr)
		return ok && selector.Sel.Name == method && isIdentExpr(selector.X, receiver)
	}
	wantParams := []string{"context.Context", "*traceDB", "*traceDBRowSink", "traceDBSchedulerAuthority", "traceDBSchedulerRunningIndex", "*traceDBSyncSpanAuthority"}
	var gotParams []string
	for _, field := range exporter.Type.Params.List {
		name := typeName(field.Type)
		if selector, ok := field.Type.(*ast.SelectorExpr); ok {
			if receiver, ok := selector.X.(*ast.Ident); ok {
				name = receiver.Name + "." + selector.Sel.Name
			}
		}
		for range field.Names {
			gotParams = append(gotParams, name)
		}
	}
	if !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("syscall authority signature=%v want=%v", gotParams, wantParams)
	}
	callCounts := func(function *ast.FuncDecl) map[string]int {
		counts := map[string]int{}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch callee := call.Fun.(type) {
			case *ast.Ident:
				counts[callee.Name]++
			case *ast.SelectorExpr:
				counts[callee.Sel.Name]++
			}
			return true
		})
		return counts
	}
	callSites := func(function *ast.FuncDecl) map[string][]*ast.CallExpr {
		sites := map[string][]*ast.CallExpr{}
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
			if name != "" {
				sites[name] = append(sites[name], call)
			}
			return true
		})
		return sites
	}
	exportCalls := callCounts(exporter)
	prepareCalls := callCounts(prepare)
	exportSites := callSites(exporter)
	prepareSites := callSites(prepare)
	for name, want := range map[string]int{
		"traceDBActivityProfile": 1, "poisonExactLane": 1, "poisonGlobally": 1,
		"submit": 1, "prepareTraceDBSyscallRow": 1, "traceDBBoundedSQLiteIntegerTransport": 5,
	} {
		if exportCalls[name] != want {
			t.Fatalf("syscall exporter call count %s=%d want=%d: %v", name, exportCalls[name], want, exportCalls)
		}
	}
	decodeArgs := map[string]int{}
	for _, call := range prepareSites["decode"] {
		if !methodReceiver(call, "profile", "decode") || len(call.Args) != 1 {
			t.Fatalf("syscall signed decoder receiver/arity drifted: %#v", call)
		}
		if ident, ok := call.Args[0].(*ast.Ident); ok {
			decodeArgs[ident.Name]++
		}
	}
	if !reflect.DeepEqual(decodeArgs, map[string]int{"itidRaw": 1, "numberRaw": 1}) {
		t.Fatalf("syscall signed decoder arguments=%v", decodeArgs)
	}
	assertMethodArgs := func(name, receiver string, want ...func(ast.Expr) bool) {
		t.Helper()
		sites := prepareSites[name]
		if len(sites) != 1 || !methodReceiver(sites[0], receiver, name) || len(sites[0].Args) != len(want) {
			t.Fatalf("syscall %s receiver/arity drifted: %+v", name, sites)
		}
		for i, predicate := range want {
			if !predicate(sites[0].Args[i]) {
				t.Fatalf("syscall %s argument %d drifted: %#v", name, i, sites[0].Args[i])
			}
		}
	}
	rowField := func(field string) func(ast.Expr) bool {
		return func(expression ast.Expr) bool { return isSelectorExpr(expression, "row", field) }
	}
	assertMethodArgs("resolveThreadSubject", "authority", rowField("EmitterITID"))
	assertMethodArgs("threadPointAllows", "authority", rowField("EmitterITID"), rowField("TS"))
	assertMethodArgs("threadClosedEndpointAllows", "authority", rowField("EmitterITID"), rowField("TS"), rowField("End"))
	assertMethodArgs("processClosedEndpointAllows", "authority", rowField("OwnerIPID"), rowField("TS"), rowField("End"))
	lookupArgs := map[string]int{}
	for _, call := range prepareSites["lookupCPUAt"] {
		if !methodReceiver(call, "running", "lookupCPUAt") || len(call.Args) != 2 || !isSelectorExpr(call.Args[0], "row", "EmitterITID") {
			t.Fatalf("syscall Running lookup receiver/identity drifted: %#v", call)
		}
		switch {
		case isSelectorExpr(call.Args[1], "row", "TS"):
			lookupArgs["start"]++
		case isSelectorExpr(call.Args[1], "row", "End"):
			lookupArgs["end"]++
		default:
			t.Fatalf("syscall Running lookup timestamp drifted: %#v", call.Args[1])
		}
	}
	if !reflect.DeepEqual(lookupArgs, map[string]int{"start": 1, "end": 1}) {
		t.Fatalf("syscall Running endpoint lookups=%v", lookupArgs)
	}
	beforeCalls := prepareSites["traceDBBeforeCaptureStart"]
	if len(beforeCalls) != 1 || len(beforeCalls[0].Args) != 2 ||
		!isSelectorExpr(beforeCalls[0].Args[0], "authority", "identities") || !isSelectorExpr(beforeCalls[0].Args[1], "row", "TS") {
		t.Fatalf("syscall capture-start gate arguments drifted: %+v", beforeCalls)
	}
	submitCalls := exportSites["submit"]
	if len(submitCalls) != 1 || !methodReceiver(submitCalls[0], "syncSpans", "submit") ||
		len(submitCalls[0].Args) != 2 || !isIdentExpr(submitCalls[0].Args[0], "ctx") {
		t.Fatalf("syscall submit authority drifted: %+v", submitCalls)
	}
	candidate, ok := submitCalls[0].Args[1].(*ast.CompositeLit)
	if !ok {
		t.Fatal("syscall submit candidate is not typed literal")
	}
	candidateFields := map[string]ast.Expr{}
	for _, element := range candidate.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			t.Fatal("syscall candidate has unkeyed field")
		}
		candidateFields[pair.Key.(*ast.Ident).Name] = pair.Value
	}
	for field, want := range map[string]string{
		"StableID": "StableID", "HeaderTID": "TID", "HeaderTGID": "TGID", "CanonicalITID": "EmitterITID",
		"OwnerIPID": "OwnerIPID", "Start": "TS", "End": "End", "StartCPU": "StartCPU", "EndCPU": "EndCPU", "Task": "Task",
	} {
		if !isSelectorExpr(candidateFields[field], "row", want) {
			t.Fatalf("syscall candidate %s no longer comes from row.%s: %#v", field, want, candidateFields[field])
		}
	}
	for _, field := range []string{"StartCPUProvenance", "EndCPUProvenance"} {
		if !isIdentExpr(candidateFields[field], "traceDBSyncSpanCPUSyscallTypedRunning") {
			t.Fatalf("syscall candidate %s provenance drifted: %#v", field, candidateFields[field])
		}
	}
	if !isIdentExpr(candidateFields["Producer"], "traceDBSyncSpanProducerSyscall") ||
		!isIdentExpr(candidateFields["StableKind"], "traceDBSyncSpanStableSyscallRowID") ||
		!isIdentExpr(candidateFields["CanonicalITIDKnown"], "true") ||
		!isIdentExpr(candidateFields["OwnerIPIDKnown"], "true") ||
		!isIdentExpr(candidateFields["NameProvenance"], "traceDBSyncSpanNameSyscallNumber") {
		t.Fatalf("syscall candidate closed fields drifted: %v", candidateFields)
	}
	prepareHandoff := exportSites["prepareTraceDBSyscallRow"]
	wantPrepareArgs := []string{"authority", "running", "profile", "stableRaw", "tsRaw", "durRaw", "numberRaw", "itidRaw"}
	if len(prepareHandoff) != 1 || len(prepareHandoff[0].Args) != len(wantPrepareArgs) {
		t.Fatalf("syscall prepare handoff arity drifted: %+v", prepareHandoff)
	}
	for i, want := range wantPrepareArgs {
		if !isIdentExpr(prepareHandoff[0].Args[i], want) {
			t.Fatalf("syscall prepare handoff arg %d drifted: got=%#v want=%s", i, prepareHandoff[0].Args[i], want)
		}
	}
	transportPairs := map[string]int{}
	for _, call := range exportSites["traceDBBoundedSQLiteIntegerTransport"] {
		if len(call.Args) != 2 {
			t.Fatalf("bounded syscall transport arity drifted: %#v", call)
		}
		left, leftOK := call.Args[0].(*ast.Ident)
		right, rightOK := call.Args[1].(*ast.Ident)
		if !leftOK || !rightOK {
			t.Fatalf("bounded syscall transport uses non-scalar locals: %#v", call)
		}
		transportPairs[left.Name+","+right.Name]++
	}
	if !reflect.DeepEqual(transportPairs, map[string]int{
		"tsTypeRaw,tsValueRaw":         1,
		"durTypeRaw,durValueRaw":       1,
		"numberTypeRaw,numberValueRaw": 1,
		"itidTypeRaw,itidValueRaw":     2,
	}) {
		t.Fatalf("bounded syscall transport pairings=%v", transportPairs)
	}
	for name, want := range map[string]int{
		"decode": 2, "resolveThreadSubject": 1, "traceDBBeforeCaptureStart": 1,
		"threadPointAllows": 1, "threadClosedEndpointAllows": 1, "processClosedEndpointAllows": 1,
		"lookupCPUAt": 2,
	} {
		if prepareCalls[name] != want {
			t.Fatalf("syscall row gate count %s=%d want=%d: %v", name, prepareCalls[name], want, prepareCalls)
		}
	}
	for _, forbidden := range []string{"traceDBAnyText", "traceDBStrictInternalID", "knownCPUAt", "addTraceDBSpanRows", "addTraceDBInstantRow", "addTraceDBAsyncSpanRows", "prepareTraceDBRenderedRow", "add"} {
		if exportCalls[forbidden] != 0 || prepareCalls[forbidden] != 0 {
			t.Fatalf("syscall closure retained forbidden bypass %s: export=%v prepare=%v", forbidden, exportCalls, prepareCalls)
		}
	}
	if len(exporter.Type.Params.List) < 3 || len(exporter.Type.Params.List[2].Names) != 1 || exporter.Type.Params.List[2].Names[0].Name != "_" {
		t.Fatal("syscall exporter regained a usable row-sink parameter")
	}
	queryAssignments := 0
	mainSQL := ""
	breaks := 0
	ast.Inspect(exporter.Body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.AssignStmt:
			for _, left := range typed.Lhs {
				ident, ok := left.(*ast.Ident)
				if !ok || ident.Name != "query" {
					continue
				}
				queryAssignments++
				if typed.Tok != token.DEFINE || len(typed.Rhs) != 1 {
					t.Fatalf("syscall query is mutated after definition: %#v", typed)
				}
				call, ok := typed.Rhs[0].(*ast.CallExpr)
				if !ok || len(call.Args) != 2 || !isIdentExpr(call.Args[1], "stableExpr") {
					t.Fatalf("syscall query is not one closed fmt.Sprintf(stableExpr): %#v", typed.Rhs[0])
				}
				callee, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || callee.Sel.Name != "Sprintf" || !isIdentExpr(callee.X, "fmt") {
					t.Fatalf("syscall query builder drifted: %#v", call.Fun)
				}
				literal, ok := call.Args[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					t.Fatal("syscall query format is not one literal")
				}
				mainSQL, _ = strconv.Unquote(literal.Value)
			}
		case *ast.BranchStmt:
			if typed.Tok == token.BREAK {
				breaks++
			}
		}
		return true
	})
	if queryAssignments != 1 || breaks != 0 {
		t.Fatalf("syscall query assignments=%d breaks=%d, want 1/0", queryAssignments, breaks)
	}
	canonical := func(value string) string { return strings.Join(strings.Fields(value), " ") }
	wantMainSQL := "SELECT %s, typeof(ts), CASE WHEN typeof(ts) = 'integer' THEN ts END, typeof(dur), CASE WHEN typeof(dur) = 'integer' THEN dur END, typeof(syscall_number), CASE WHEN typeof(syscall_number) = 'integer' THEN syscall_number END, typeof(itid), CASE WHEN typeof(itid) = 'integer' THEN itid END FROM syscall"
	if canonical(mainSQL) != wantMainSQL {
		t.Fatalf("syscall main SQL drifted:\ngot  %q\nwant %q", canonical(mainSQL), wantMainSQL)
	}
	queryContextKinds := map[string]int{}
	for _, call := range exportSites["QueryContext"] {
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			t.Fatalf("syscall QueryContext is not a selector: %#v", call.Fun)
		}
		dbSelector, dbOK := selector.X.(*ast.SelectorExpr)
		if !dbOK || selector.Sel.Name != "QueryContext" || !isIdentExpr(dbSelector.X, "tdb") ||
			dbSelector.Sel.Name != "db" || len(call.Args) != 2 || !isIdentExpr(call.Args[0], "ctx") {
			t.Fatalf("syscall QueryContext receiver/arity drifted: %#v", call)
		}
		switch query := call.Args[1].(type) {
		case *ast.Ident:
			if query.Name != "query" {
				t.Fatalf("syscall QueryContext uses unpinned identifier %s", query.Name)
			}
			queryContextKinds["main"]++
		case *ast.BasicLit:
			value, err := strconv.Unquote(query.Value)
			want := "SELECT typeof(itid), CASE WHEN typeof(itid) = 'integer' THEN itid END FROM syscall"
			if err != nil || canonical(value) != want {
				t.Fatalf("syscall missing-schema SQL drifted: %q", value)
			}
			queryContextKinds["missing-schema"]++
		default:
			t.Fatalf("syscall QueryContext gained dynamic SQL: %#v", call.Args[1])
		}
	}
	if !reflect.DeepEqual(queryContextKinds, map[string]int{"main": 1, "missing-schema": 1}) {
		t.Fatalf("syscall QueryContext closure=%v", queryContextKinds)
	}
	var mainLoop *ast.ForStmt
	submitPos := submitCalls[0].Pos()
	ast.Inspect(exporter.Body, func(node ast.Node) bool {
		loop, ok := node.(*ast.ForStmt)
		if ok && loop.Body.Pos() < submitPos && submitPos < loop.Body.End() {
			mainLoop = loop
			return false
		}
		return true
	})
	if mainLoop == nil {
		t.Fatal("syscall submit is no longer inside the physical-row scan")
	}
	preparePos := exportSites["prepareTraceDBSyscallRow"][0].Pos()
	rejectionPos := token.NoPos
	mainContinues := 0
	ast.Inspect(mainLoop.Body, func(node ast.Node) bool {
		if branch, ok := node.(*ast.BranchStmt); ok && branch.Tok == token.CONTINUE {
			mainContinues++
		}
		return true
	})
	ast.Inspect(mainLoop.Body, func(node ast.Node) bool {
		ifStatement, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}
		hasPoison, hasContinue := false, false
		ast.Inspect(ifStatement.Body, func(child ast.Node) bool {
			switch typed := child.(type) {
			case *ast.CallExpr:
				if ident, ok := typed.Fun.(*ast.Ident); ok && ident.Name == "poisonRejected" {
					hasPoison = true
				}
			case *ast.BranchStmt:
				hasContinue = hasContinue || typed.Tok == token.CONTINUE
			}
			return true
		})
		if hasPoison && hasContinue {
			rejectionPos = ifStatement.Pos()
			return false
		}
		return true
	})
	if rejectionPos == token.NoPos || mainContinues != 1 || !(preparePos < rejectionPos && rejectionPos < submitPos) {
		t.Fatalf("syscall rejected-row poison no longer dominates submit: prepare=%d reject=%d submit=%d continues=%d",
			preparePos, rejectionPos, submitPos, mainContinues)
	}
	forbiddenSQL := []string{" WHERE ", "COALESCE(", "CAST(", " DISTINCT ", " GROUP ", " HAVING ", " LIMIT ", " JOIN ", " UNION "}
	sqlQueries := 0
	typeofGuards := 0
	ast.Inspect(exporter.Body, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil || !strings.Contains(strings.ToUpper(value), "SELECT") {
			return true
		}
		sqlQueries++
		upper := " " + strings.ToUpper(strings.Join(strings.Fields(value), " ")) + " "
		for _, forbidden := range forbiddenSQL {
			if strings.Contains(upper, forbidden) {
				t.Fatalf("syscall SQL contains forbidden source-side transform %q: %q", forbidden, value)
			}
		}
		if strings.Contains(upper, " ELSE ") || !strings.Contains(upper, "TYPEOF(ITID)") ||
			!strings.Contains(upper, "CASE WHEN TYPEOF(ITID) = 'INTEGER' THEN ITID END") {
			t.Fatalf("syscall SQL lost bounded typeof transport: %q", value)
		}
		typeofGuards += strings.Count(upper, "CASE WHEN TYPEOF(")
		return true
	})
	if sqlQueries != 2 || typeofGuards != 5 {
		t.Fatalf("syscall bounded SQL envelope queries=%d typeof_guards=%d, want 2/5", sqlQueries, typeofGuards)
	}
	legacyIdentifiers := map[string]bool{"traceDBSyncSpanCPULegacyUnverified": false, "traceDBSyncSpanCPUSyscallTypedRunning": false}
	ast.Inspect(exporter.Body, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok {
			if _, tracked := legacyIdentifiers[ident.Name]; tracked {
				legacyIdentifiers[ident.Name] = true
			}
		}
		return true
	})
	if legacyIdentifiers["traceDBSyncSpanCPULegacyUnverified"] || !legacyIdentifiers["traceDBSyncSpanCPUSyscallTypedRunning"] {
		t.Fatalf("syscall CPU provenance drifted: %v", legacyIdentifiers)
	}
	dispatches := 0
	ast.Inspect(extended.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, ok := call.Fun.(*ast.Ident)
		if !ok || callee.Name != "exportTraceDBSyscall" {
			return true
		}
		dispatches++
		want := []string{"ctx", "tdb", "sink", "authority", "lifecycleRunning", "syncSpans"}
		if len(call.Args) != len(want) {
			t.Fatalf("syscall production dispatch args=%d want=%d", len(call.Args), len(want))
		}
		for i, argument := range call.Args {
			ident, ok := argument.(*ast.Ident)
			if !ok || ident.Name != want[i] {
				t.Fatalf("syscall dispatch arg %d=%T want=%s", i, argument, want[i])
			}
		}
		return true
	})
	if dispatches != 1 {
		t.Fatalf("syscall production dispatches=%d want=1", dispatches)
	}
}
