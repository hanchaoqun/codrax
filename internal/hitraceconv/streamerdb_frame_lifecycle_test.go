package hitraceconv

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func traceDBFrameB2CurrentStatements(threadName string, runningRows, frameRows []string) []string {
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'old-process')",
		"INSERT INTO process VALUES (2, 100, 'new-process')",
		"INSERT INTO process VALUES (3, 300, 'other-process')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		fmt.Sprintf("INSERT INTO thread VALUES (1, 101, 1, '%s', 0, 0, 1)", threadName),
		"INSERT INTO thread VALUES (2, 101, 2, 'new-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (3, 301, 3, 'other-thread', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
	}
	statements = append(statements, runningRows...)
	statements = append(statements,
		"CREATE TABLE frame_slice (id, ts, dur, type, type_desc, vsync, flag, ipid, itid)")
	return append(statements, frameRows...)
}

func traceDBFrameB2Export(t *testing.T, statements []string, lifecycle traceDBLifecycleIndex, complete bool,
	mutateIntegrity func(*traceDBRunningIntegrity),
) (TraceDBCoverage, []string) {
	t.Helper()
	path := createTraceDBFixture(t, statements)
	ctx := context.Background()
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
	if mutateIntegrity != nil {
		mutateIntegrity(&integrity)
	}
	profile, profileSource, err := traceDBActivityProfile(ctx, tdb.db, "frame_slice")
	if err != nil {
		t.Fatal(err)
	}
	authority := newTraceDBSchedulerAuthority(identities, traceDBLifecycleCollection{
		Lifecycle:          lifecycle,
		FrameProfile:       profile,
		FrameProfileSource: profileSource,
		CreationComplete:   complete,
		TerminalComplete:   complete,
		ActivityComplete:   complete,
	})
	running := newTraceDBSchedulerRunningIndex(authority, intervals, integrity, nil)
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := exportTraceDBFrameSlice(ctx, tdb, sink, authority, running)
	if err != nil {
		t.Fatalf("export frame lifecycle fixture: %v coverage=%+v", err, coverage)
	}
	rows := append([]traceDBStoredRow(nil), sink.rows...)
	sortTraceDBStoredRows(rows)
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, row.line)
	}
	return coverage, lines
}

func traceDBFrameB2Cut(cut int64, threadCut, processCut bool, newITID, newIPID int64) traceDBLifecycleIndex {
	lifecycle := traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{}, ByPID: map[int64]traceDBLifecycleLane{}}
	boundary := traceDBLifecycleBoundary{TS: cut, NewITID: newITID, NewIPID: newIPID}
	if threadCut {
		lifecycle.ByTID[101] = traceDBLifecycleLane{Cuts: []traceDBLifecycleBoundary{boundary}}
	}
	if processCut {
		lifecycle.ByPID[100] = traceDBLifecycleLane{Cuts: []traceDBLifecycleBoundary{boundary}}
	}
	return lifecycle
}

func traceDBFrameB2Body(lines []string) string {
	return strings.Join(lines, "\n")
}

func TestTraceDBFrameClosedEndpointLifecycleMatrix(t *testing.T) {
	tests := []struct {
		name       string
		itid       int64
		ipid       int64
		ts         int64
		dur        int64
		lifecycle  traceDBLifecycleIndex
		complete   bool
		wantEmit   int
		wantReason string
	}{
		{name: "clean", itid: 1, ipid: 1, ts: 1000, dur: 1000, lifecycle: traceDBFrameB2Cut(0, false, false, 0, 0), complete: true, wantEmit: 2},
		{name: "thread cut interior", itid: 1, ipid: 1, ts: 1000, dur: 1000, lifecycle: traceDBFrameB2Cut(1500, true, false, 2, 2), complete: true, wantReason: "lifecycle_rejected_frame_endpoint=1"},
		{name: "process cut interior", itid: 1, ipid: 1, ts: 1000, dur: 1000, lifecycle: traceDBFrameB2Cut(1500, false, true, 2, 2), complete: true, wantReason: "lifecycle_rejected_frame_endpoint=1"},
		{name: "closed end cut", itid: 1, ipid: 1, ts: 1000, dur: 1000, lifecycle: traceDBFrameB2Cut(2000, true, true, 2, 2), complete: true, wantReason: "lifecycle_rejected_frame_endpoint=1"},
		{name: "same identity closed end cut", itid: 1, ipid: 1, ts: 1000, dur: 1000, lifecycle: traceDBFrameB2Cut(2000, true, true, 1, 1), complete: true, wantReason: "lifecycle_rejected_frame_endpoint=1"},
		{name: "future cut", itid: 1, ipid: 1, ts: 1000, dur: 1000, lifecycle: traceDBFrameB2Cut(2001, true, true, 2, 2), complete: true, wantEmit: 2},
		{name: "new generation at start", itid: 2, ipid: 2, ts: 1000, dur: 1000, lifecycle: traceDBFrameB2Cut(1000, true, true, 2, 2), complete: true, wantEmit: 2},
		{name: "old generation at start", itid: 1, ipid: 1, ts: 1000, dur: 1000, lifecycle: traceDBFrameB2Cut(1000, true, true, 2, 2), complete: true, wantReason: "lifecycle_rejected_frame_endpoint=1"},
		{name: "incomplete authority", itid: 1, ipid: 1, ts: 1000, dur: 1000, lifecycle: traceDBFrameB2Cut(0, false, false, 0, 0), wantReason: "lifecycle_rejected_frame_endpoint=1"},
		{name: "global poison interior", itid: 1, ipid: 1, ts: 1000, dur: 1000, lifecycle: traceDBLifecycleIndex{GlobalPoison: []int64{1500}}, complete: true, wantReason: "lifecycle_rejected_frame_endpoint=1"},
		{name: "thread poison interior", itid: 1, ipid: 1, ts: 1000, dur: 1000, lifecycle: traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{101: {PoisonPoints: []int64{1500}}}}, complete: true, wantReason: "lifecycle_rejected_frame_endpoint=1"},
		{name: "process poison interior", itid: 1, ipid: 1, ts: 1000, dur: 1000, lifecycle: traceDBLifecycleIndex{ByPID: map[int64]traceDBLifecycleLane{100: {PoisonPoints: []int64{1500}}}}, complete: true, wantReason: "lifecycle_rejected_frame_endpoint=1"},
		{name: "global taint", itid: 1, ipid: 1, ts: 1000, dur: 1000, lifecycle: traceDBLifecycleIndex{GlobalTaint: true}, complete: true, wantReason: "lifecycle_rejected_frame_endpoint=1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			end := test.ts + test.dur
			runningRows := []string{
				fmt.Sprintf("INSERT INTO thread_state VALUES (%d, %d, 1, 3, 'Running')", test.itid, test.ts),
				fmt.Sprintf("INSERT INTO thread_state VALUES (%d, %d, 1, 7, 'Running')", test.itid, end),
			}
			frameRows := []string{fmt.Sprintf(
				"INSERT INTO frame_slice VALUES (1, %d, %d, 0, 'actural', 1, 1, %d, %d)",
				test.ts, test.dur, test.ipid, test.itid)}
			coverage, lines := traceDBFrameB2Export(t,
				traceDBFrameB2CurrentStatements("old-thread", runningRows, frameRows), test.lifecycle, test.complete, nil)
			if coverage.RowsEmitted != test.wantEmit {
				t.Fatalf("RowsEmitted=%d want=%d coverage=%+v lines=%q", coverage.RowsEmitted, test.wantEmit, coverage, lines)
			}
			if test.wantReason != "" && !strings.Contains(coverage.Skipped, test.wantReason) {
				t.Fatalf("coverage missing %q: %+v", test.wantReason, coverage)
			}
			if test.wantEmit == 0 && strings.Contains(traceDBFrameB2Body(lines), "hconv-frame-1") {
				t.Fatalf("rejected frame leaked a partial endpoint: %q", lines)
			}
		})
	}
}

func TestTraceDBFrameLifecycleRejectionIsRowAndLaneLocal(t *testing.T) {
	lifecycle := traceDBFrameB2Cut(15000, true, true, 2, 2)
	statements := traceDBFrameB2CurrentStatements("old-thread", []string{
		"INSERT INTO thread_state VALUES (1, 1000, 9001, 1, 'Running')",
		"INSERT INTO thread_state VALUES (3, 1000, 9001, 3, 'Running')",
	}, []string{
		"INSERT INTO frame_slice VALUES (1, 10000, 10000, 0, 'actural', 1, 1, 1, 1)",
		"INSERT INTO frame_slice VALUES (2, 1000, 9000, 0, 'actural', 2, 1, 1, 1)",
		"INSERT INTO frame_slice VALUES (3, 1000, 9000, 0, 'actural', 3, 1, 3, 3)",
	})
	coverage, lines := traceDBFrameB2Export(t, statements, lifecycle, true, nil)
	body := traceDBFrameB2Body(lines)
	if coverage.RowsEmitted != 4 || !strings.Contains(coverage.Skipped, "lifecycle_rejected_frame_endpoint=1") ||
		strings.Contains(body, "hconv-frame-1") || !strings.Contains(body, "hconv-frame-2") || !strings.Contains(body, "hconv-frame-3") {
		t.Fatalf("frame lifecycle locality/atomicity mismatch: coverage=%+v body=%q", coverage, body)
	}
}

func TestTraceDBFrameUsesExactEndCPU(t *testing.T) {
	statements := func(withExactEnd bool) []string {
		runningRows := []string{"INSERT INTO thread_state VALUES (1, 1000, 1000, 3, 'Running')"}
		if withExactEnd {
			runningRows = append(runningRows, "INSERT INTO thread_state VALUES (1, 2000, 1, 7, 'Running')")
		}
		return traceDBFrameB2CurrentStatements("old-thread", runningRows, []string{
			"INSERT INTO frame_slice VALUES (1, 1000, 1000, 0, 'actural', 1, 1, 1, 1)",
		})
	}

	t.Run("endpoint CPUs come from exact physical instants", func(t *testing.T) {
		coverage, lines := traceDBFrameB2Export(t, statements(true), traceDBLifecycleIndex{}, true, nil)
		startExact, endExact := false, false
		for _, line := range lines {
			startExact = startExact || strings.Contains(line, "[003]") && strings.Contains(line, "tracing_mark_write: S|100|")
			endExact = endExact || strings.Contains(line, "[007]") && strings.Contains(line, "tracing_mark_write: F|100|")
		}
		if coverage.RowsEmitted != 2 || coverage.Skipped != "" || !startExact || !endExact {
			t.Fatalf("exact endpoint CPU provenance mismatch: coverage=%+v lines=%q", coverage, lines)
		}
	})

	t.Run("end minus one cannot rescue a missing exact end", func(t *testing.T) {
		coverage, lines := traceDBFrameB2Export(t, statements(false), traceDBLifecycleIndex{}, true, nil)
		if coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "unknown_end_cpu=1") ||
			strings.Contains(traceDBFrameB2Body(lines), "hconv-frame-1") {
			t.Fatalf("End-1 witness rescued an unknown exact End: coverage=%+v lines=%q", coverage, lines)
		}
	})
}

func TestTraceDBFrameTypedRunningAntiRescueIsOrderIndependent(t *testing.T) {
	for _, kind := range []string{"lifecycle", "source"} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", kind, reverse), func(t *testing.T) {
				bad := "INSERT INTO thread_state VALUES (1, 14000, 2000, 2, 'Running')"
				wantReason := "lifecycle_rejected_running_cpu_witness=1"
				lifecycle := traceDBFrameB2Cut(15000, true, true, 2, 2)
				if kind == "source" {
					bad = "INSERT INTO thread_state VALUES (1, 14000, 2000, 5000, 'Running')"
					wantReason = "tainted_running_cpu_witness=1"
					lifecycle = traceDBLifecycleIndex{}
				}
				valid := "INSERT INTO thread_state VALUES (1, 1000, 10000, 1, 'Running')"
				if reverse {
					bad, valid = valid, bad
				}
				statements := traceDBFrameB2CurrentStatements("old-thread", []string{
					bad,
					valid,
					"INSERT INTO thread_state VALUES (3, 1000, 10000, 3, 'Running')",
				}, []string{
					"INSERT INTO frame_slice VALUES (1, 2000, 1000, 0, 'actural', 1, 1, 1, 1)",
					"INSERT INTO frame_slice VALUES (2, 2000, 1000, 0, 'actural', 2, 1, 3, 3)",
				})
				coverage, lines := traceDBFrameB2Export(t, statements, lifecycle, true, nil)
				body := traceDBFrameB2Body(lines)
				if coverage.RowsEmitted != 2 || !strings.Contains(coverage.Skipped, wantReason) ||
					strings.Contains(body, "hconv-frame-1") || !strings.Contains(body, "hconv-frame-2") {
					t.Fatalf("typed Running anti-rescue/locality mismatch: coverage=%+v body=%q", coverage, body)
				}
			})
		}
	}
}

func traceDBFrameB2HighIdentityStatements(current bool, runningRows, frameRows []string) []string {
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (4294967294, 500, 'high-process')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (4294967293, 501, 4294967294, 'high-thread', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
	}
	statements = append(statements, runningRows...)
	if current {
		statements = append(statements,
			"CREATE TABLE frame_slice (id, ts, dur, type, type_desc, vsync, flag, ipid, itid)")
	} else {
		statements = append(statements,
			"CREATE TABLE frame_slice (ts, dur, type_desc, vsync, flag, ipid, itid)")
	}
	return append(statements, frameRows...)
}

func TestTraceDBFrameCurrentProfileProjectsAllThreeIdentityDomains(t *testing.T) {
	statements := traceDBFrameB2HighIdentityStatements(true, []string{
		"INSERT INTO thread_state VALUES (4294967293, 0, 20000, 4, 'Running')",
	}, []string{
		"INSERT INTO frame_slice VALUES (-1, 1000, 1000, 0, 'actural', 1, 1, -2, -3)",
		"INSERT INTO frame_slice VALUES (4294967294, 3000, 1000, 0, 'actural', 2, 1, -2, -3)",
		"INSERT INTO frame_slice VALUES (1, 5000, 1000, 0, 'actural', 3, 1, 4294967294, -3)",
		"INSERT INTO frame_slice VALUES (2, 7000, 1000, 0, 'actural', 4, 1, -2, 4294967293)",
		"INSERT INTO frame_slice VALUES (3, 9000, 1000, 0, 'actural', 5, 1, -1, -3)",
		"INSERT INTO frame_slice VALUES (4, 11000, 1000, 0, 'actural', 6, 1, -2, -1)",
	})
	coverage, lines := traceDBFrameB2Export(t, statements, traceDBLifecycleIndex{}, true, nil)
	body := traceDBFrameB2Body(lines)
	for _, want := range []string{"invalid_row_identity=1", "invalid_owner_ipid=2", "invalid_emitter_itid=2"} {
		if !strings.Contains(coverage.Skipped, want) {
			t.Fatalf("current profile missing typed rejection %q: %+v", want, coverage)
		}
	}
	if coverage.RowsEmitted != 2 || !strings.Contains(body, "hconv-frame-4294967295") ||
		!strings.Contains(coverage.FieldSources["schema_profile"], "current frame_slice") {
		t.Fatalf("current ID/IPID/ITID projection mismatch: coverage=%+v body=%q", coverage, body)
	}
}

func TestTraceDBFrameCurrentStableIdentityDuplicateAndCanonicalOrder(t *testing.T) {
	t.Run("decoded UINT32_MAX duplicate cohort rejects both rows", func(t *testing.T) {
		statements := traceDBFrameB2CurrentStatements("old-thread", nil, []string{
			"INSERT INTO frame_slice VALUES (-1, 1000, 1000, 0, 'actural', 1, 1, 1, 1)",
			"INSERT INTO frame_slice VALUES (-1, 3000, 1000, 0, 'actural', 2, 1, 1, 1)",
		})
		coverage, lines := traceDBFrameB2Export(t, statements, traceDBLifecycleIndex{}, true, nil)
		if coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "duplicate_row_identity=2") || len(lines) != 0 {
			t.Fatalf("decoded stable-ID duplicate escaped: coverage=%+v lines=%q", coverage, lines)
		}
	})

	t.Run("same timestamp order follows canonical uint32", func(t *testing.T) {
		statements := traceDBFrameB2CurrentStatements("old-thread", []string{
			"INSERT INTO thread_state VALUES (1, 0, 5000, 1, 'Running')",
		}, []string{
			"INSERT INTO frame_slice VALUES (-2147483648, 1000, 1000, 0, 'actural', 2, 1, 1, 1)",
			"INSERT INTO frame_slice VALUES (2147483647, 1000, 1000, 0, 'actural', 1, 1, 1, 1)",
		})
		coverage, lines := traceDBFrameB2Export(t, statements, traceDBLifecycleIndex{}, true, nil)
		body := traceDBFrameB2Body(lines)
		low := strings.Index(body, "S|100|FrameActual-1|hconv-frame-2147483647")
		high := strings.Index(body, "S|100|FrameActual-2|hconv-frame-2147483648")
		if coverage.RowsEmitted != 4 || low < 0 || high < 0 || low >= high {
			t.Fatalf("canonical current frame order mismatch: coverage=%+v low=%d high=%d body=%q", coverage, low, high, body)
		}
	})
}

func TestTraceDBFrameLegacyProfileKeepsCanonicalOwnerIdentities(t *testing.T) {
	statements := traceDBFrameB2HighIdentityStatements(false, []string{
		"INSERT INTO thread_state VALUES (4294967293, 0, 10000, 4, 'Running')",
	}, []string{
		"INSERT INTO frame_slice VALUES (1000, 1000, 'actural', 1, 1, 4294967294, 4294967293)",
		"INSERT INTO frame_slice VALUES (3000, 1000, 'actural', 2, 1, -2, 4294967293)",
		"INSERT INTO frame_slice VALUES (5000, 1000, 'actural', 3, 1, 4294967294, -3)",
		"INSERT INTO frame_slice VALUES (7000, 1000, 'actural', 4, 1, -1, 4294967293)",
		"INSERT INTO frame_slice VALUES (9000, 1000, 'actural', 5, 1, 4294967294, -1)",
	})
	coverage, lines := traceDBFrameB2Export(t, statements, traceDBLifecycleIndex{}, true, nil)
	body := traceDBFrameB2Body(lines)
	if coverage.RowsEmitted != 2 || !strings.Contains(coverage.Skipped, "invalid_owner_ipid=2") ||
		!strings.Contains(coverage.Skipped, "invalid_emitter_itid=2") ||
		!strings.Contains(body, "hconv-frame-1") || !strings.Contains(coverage.FieldSources["schema_profile"], "legacy frame_slice") {
		t.Fatalf("legacy canonical identity profile mismatch: coverage=%+v body=%q", coverage, body)
	}
}

func TestTraceDBFrameVSyncUsesTheCollectorSelectedProfile(t *testing.T) {
	t.Run("current signed projection", func(t *testing.T) {
		statements := traceDBFrameB2CurrentStatements("old-thread", []string{
			"INSERT INTO thread_state VALUES (1, 0, 10000, 4, 'Running')",
		}, []string{
			"INSERT INTO frame_slice VALUES (1, 1000, 1000, 0, 'actural', -2, 1, 1, 1)",
			"INSERT INTO frame_slice VALUES (2, 3000, 1000, 0, 'actural', 4294967294, 1, 1, 1)",
			"INSERT INTO frame_slice VALUES (3, 5000, 1000, 0, 'actural', -1, 1, 1, 1)",
		})
		coverage, lines := traceDBFrameB2Export(t, statements, traceDBLifecycleIndex{}, true, nil)
		body := traceDBFrameB2Body(lines)
		if coverage.RowsEmitted != 2 || !strings.Contains(coverage.Skipped, "invalid_vsync=2") ||
			!strings.Contains(body, "FrameActual-4294967294") || strings.Contains(body, "hconv-frame-2") || strings.Contains(body, "hconv-frame-3") {
			t.Fatalf("current VSync projection mismatch: coverage=%+v body=%q", coverage, body)
		}
	})

	t.Run("legacy canonical", func(t *testing.T) {
		statements := traceDBFrameB2HighIdentityStatements(false, []string{
			"INSERT INTO thread_state VALUES (4294967293, 0, 10000, 4, 'Running')",
		}, []string{
			"INSERT INTO frame_slice VALUES (1000, 1000, 'actural', 4294967294, 1, 4294967294, 4294967293)",
			"INSERT INTO frame_slice VALUES (3000, 1000, 'actural', -2, 1, 4294967294, 4294967293)",
			"INSERT INTO frame_slice VALUES (5000, 1000, 'actural', -1, 1, 4294967294, 4294967293)",
		})
		coverage, lines := traceDBFrameB2Export(t, statements, traceDBLifecycleIndex{}, true, nil)
		body := traceDBFrameB2Body(lines)
		if coverage.RowsEmitted != 2 || !strings.Contains(coverage.Skipped, "invalid_vsync=2") ||
			!strings.Contains(body, "FrameActual-4294967294") {
			t.Fatalf("legacy VSync profile mismatch: coverage=%+v body=%q", coverage, body)
		}
	})
}

func TestTraceDBFrameThreadRenameIsDisplayOnly(t *testing.T) {
	load := func(t *testing.T, name string) (TraceDBCoverage, []string) {
		t.Helper()
		return traceDBFrameB2Export(t, traceDBFrameB2CurrentStatements(name, []string{
			"INSERT INTO thread_state VALUES (1, 1000, 1001, 5, 'Running')",
		}, []string{
			"INSERT INTO frame_slice VALUES (1, 1000, 1000, 0, 'actural', 1, 1, 1, 1)",
		}), traceDBLifecycleIndex{}, true, nil)
	}
	beforeCoverage, before := load(t, "before-name")
	afterCoverage, after := load(t, "after-name")
	if beforeCoverage.RowsEmitted != 2 || afterCoverage.RowsEmitted != 2 ||
		beforeCoverage.Skipped != afterCoverage.Skipped || len(before) != len(after) {
		t.Fatalf("rename changed hard admission: before=%+v %q after=%+v %q", beforeCoverage, before, afterCoverage, after)
	}
	for i := range before {
		beforeEnvelope := strings.Index(before[i], "[")
		afterEnvelope := strings.Index(after[i], "[")
		if beforeEnvelope < 0 || afterEnvelope < 0 || before[i][beforeEnvelope:] != after[i][afterEnvelope:] {
			t.Fatalf("rename changed CPU/time/wire endpoint %d: before=%q after=%q", i, before[i], after[i])
		}
	}
	if !strings.Contains(traceDBFrameB2Body(before), "before-name") || !strings.Contains(traceDBFrameB2Body(after), "after-name") {
		t.Fatalf("rename display metadata was not refreshed: before=%q after=%q", before, after)
	}
}

func TestTraceDBFrameRejectsKernelProcessOwner(t *testing.T) {
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 0, 'kernel')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 101, 1, 'kernel-worker', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (1, 1000, 1001, 1, 'Running')",
		"CREATE TABLE frame_slice (id, ts, dur, type, type_desc, vsync, flag, ipid, itid)",
		"INSERT INTO frame_slice VALUES (1, 1000, 1000, 0, 'actural', 1, 1, 1, 1)",
	}
	coverage, lines := traceDBFrameB2Export(t, statements, traceDBLifecycleIndex{}, true, nil)
	if coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "unresolved_owner_process=1") || len(lines) != 0 {
		t.Fatalf("PID0 owner borrowed the scheduler kernel exception: coverage=%+v lines=%q", coverage, lines)
	}
}
