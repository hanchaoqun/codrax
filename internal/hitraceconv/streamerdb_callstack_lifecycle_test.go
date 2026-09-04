package hitraceconv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func traceDBCallstackAuthorityStatements(runningRows, callstackRows []string) []string {
	statements := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'old-process')",
		"INSERT INTO process VALUES (2, 100, 'new-process')",
		"INSERT INTO process VALUES (3, 300, 'other-process')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 101, 1, 'old-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 101, 2, 'new-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (3, 301, 3, 'other-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (4, 104, 1, 'migrated-thread', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
	}
	statements = append(statements, runningRows...)
	statements = append(statements, "CREATE TABLE callstack (id, ts, dur, itid, callid, name, flag, cookie, chainId, depth)")
	statements = append(statements, callstackRows...)
	return statements
}

func exportTraceDBCallstackAuthorityFixture(t *testing.T, statements []string, lifecycle traceDBLifecycleIndex,
	complete bool, mutateIntegrity func(*traceDBRunningIntegrity),
) (TraceDBCoverage, string) {
	t.Helper()
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	tdb.sourceNameInventory = &traceDBSourceNameInventory{
		RawDecode: TraceDBCoverage{
			Found: true,
			Metadata: map[string]string{
				"decode_state":              "strict_target_ledger_complete",
				"target_first_timestamp_ns": "100",
			},
		},
	}
	identities, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	intervals, integrity, _, err := tdb.loadRunningIntervals(context.Background(), identities)
	if err != nil {
		t.Fatal(err)
	}
	if mutateIntegrity != nil {
		mutateIntegrity(&integrity)
	}
	authority := traceDBSchedulerAuthority{
		identities:  identities,
		lifecycle:   lifecycle,
		initialized: true,
		complete:    complete,
	}
	running := newTraceDBSchedulerRunningIndex(authority, intervals, integrity, nil)
	sink, err := newTraceDBRowSink(t.TempDir(), 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, err := exportTraceDBCallstack(context.Background(), tdb, sink, authority, running, syncSpans)
	if err != nil {
		t.Fatalf("export callstack fixture: %v coverage=%+v", err, coverage)
	}
	items, _, _ := finalizeTraceDBTestSyncSpans(t, sink, syncSpans, []TraceDBCoverage{coverage})
	coverage = items[0]
	rows := append([]traceDBStoredRow(nil), sink.rows...)
	sortTraceDBStoredRows(rows)
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, row.line)
	}
	return coverage, strings.Join(lines, "\n")
}

func TestTraceDBCallstackReportsZeroStartAndLongestAcceptedSpanWithoutChangingAdmission(t *testing.T) {
	coverage, body := exportTraceDBCallstackAuthorityFixture(t,
		traceDBCallstackAuthorityStatements(
			[]string{"INSERT INTO thread_state VALUES (1, 0, 2000000001, 3, 'Running')"},
			[]string{
				"INSERT INTO callstack VALUES (1, 0, 2000000000, 1, NULL, 'window-covering', '', NULL, NULL, 0)",
				"INSERT INTO callstack VALUES (2, 1000, 100000000, 1, NULL, 'shorter', '', NULL, NULL, 1)",
			}),
		traceDBLifecycleIndex{}, true, nil)
	if coverage.Metrics["source_rows_accepted_start_timestamp_zero"] != 1 ||
		coverage.Metrics["source_rows_accepted_before_raw_target_first_timestamp"] != 1 ||
		coverage.Metrics["source_rows_accepted_duration_ge_100ms"] != 2 ||
		coverage.Metrics["source_rows_accepted_duration_ge_1s"] != 1 ||
		coverage.Metadata["raw_target_first_timestamp_ns"] != "100" ||
		!strings.Contains(coverage.Metadata["longest_accepted_span_witness"],
			`row_id=1/start_ns=0/end_ns=2000000000/duration_ns=2000000000/header_tid=101/name="window-covering"`) ||
		!strings.Contains(body, "window-covering") {
		t.Fatalf("callstack zero/long diagnostics changed or suppressed admission: coverage=%+v body=%q",
			coverage, body)
	}
}

func traceDBCallstackCutLifecycle(cut int64, threadCut, processCut bool, newITID, newIPID int64) traceDBLifecycleIndex {
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

func TestTraceDBCallstackSyncLifecycleBoundaryMatrix(t *testing.T) {
	tests := []struct {
		name       string
		itid       int64
		dur        int64
		runningTS  int64
		lifecycle  traceDBLifecycleIndex
		complete   bool
		wantEmit   int
		wantReason string
	}{
		{name: "clean positive", itid: 1, dur: 1000, runningTS: 900, lifecycle: traceDBCallstackCutLifecycle(0, false, false, 0, 0), complete: true, wantEmit: 2},
		{name: "thread cut interior", itid: 1, dur: 1000, runningTS: 900, lifecycle: traceDBCallstackCutLifecycle(1500, true, false, 2, 2), complete: true, wantReason: "lifecycle_rejected_sync_closed_interval=1"},
		{name: "process cut interior", itid: 1, dur: 1000, runningTS: 900, lifecycle: traceDBCallstackCutLifecycle(1500, false, true, 2, 2), complete: true, wantReason: "lifecycle_rejected_sync_closed_interval=1"},
		{name: "closed end cut", itid: 1, dur: 1000, runningTS: 900, lifecycle: traceDBCallstackCutLifecycle(2000, true, true, 2, 2), complete: true, wantReason: "lifecycle_rejected_sync_closed_interval=1"},
		{name: "same identity cut at closed end", itid: 1, dur: 1000, runningTS: 900, lifecycle: traceDBCallstackCutLifecycle(2000, true, true, 1, 1), complete: true, wantReason: "lifecycle_rejected_sync_closed_interval=1"},
		{name: "future cut", itid: 1, dur: 1000, runningTS: 900, lifecycle: traceDBCallstackCutLifecycle(2001, true, true, 2, 2), complete: true, wantEmit: 2},
		{name: "new generation at start", itid: 2, dur: 1000, runningTS: 1000, lifecycle: traceDBCallstackCutLifecycle(1000, true, true, 2, 2), complete: true, wantEmit: 2},
		{name: "old generation at start", itid: 1, dur: 1000, runningTS: 900, lifecycle: traceDBCallstackCutLifecycle(1000, true, true, 2, 2), complete: true, wantReason: "lifecycle_rejected_sync_closed_interval=1"},
		{name: "clean zero", itid: 1, dur: 0, runningTS: 1000, lifecycle: traceDBCallstackCutLifecycle(0, false, false, 0, 0), complete: true, wantEmit: 2},
		{name: "new zero at cut", itid: 2, dur: 0, runningTS: 1000, lifecycle: traceDBCallstackCutLifecycle(1000, true, true, 2, 2), complete: true, wantEmit: 2},
		{name: "old zero at cut", itid: 1, dur: 0, runningTS: 1000, lifecycle: traceDBCallstackCutLifecycle(1000, true, true, 2, 2), complete: true, wantReason: "lifecycle_rejected_sync_point=1"},
		{name: "incomplete authority", itid: 1, dur: 1000, runningTS: 900, lifecycle: traceDBCallstackCutLifecycle(0, false, false, 0, 0), wantReason: "lifecycle_rejected_sync_closed_interval=1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runningDur := int64(1101)
			if test.dur == 0 {
				runningDur = 1
			}
			statements := traceDBCallstackAuthorityStatements(
				[]string{fmt.Sprintf("INSERT INTO thread_state VALUES (%d, %d, %d, 1, 'Running')", test.itid, test.runningTS, runningDur)},
				[]string{fmt.Sprintf("INSERT INTO callstack VALUES (1, 1000, %d, %d, NULL, 'sync', '', NULL, NULL, 0)", test.dur, test.itid)},
			)
			coverage, body := exportTraceDBCallstackAuthorityFixture(t, statements, test.lifecycle, test.complete, nil)
			if coverage.RowsEmitted != test.wantEmit {
				t.Fatalf("RowsEmitted=%d want %d coverage=%+v body=%q", coverage.RowsEmitted, test.wantEmit, coverage, body)
			}
			if test.wantReason != "" && !strings.Contains(coverage.Skipped, test.wantReason) {
				t.Fatalf("coverage missing %q: %+v", test.wantReason, coverage)
			}
			if test.name == "clean positive" &&
				(!strings.Contains(coverage.FieldSources["cpu"], "cpu_status=unavailable") ||
					!strings.Contains(coverage.FieldSources["cpu"], "never fabricate CPU 0") ||
					!strings.Contains(coverage.FieldSources["lifecycle"], "closed thread/process") ||
					!strings.Contains(coverage.FieldSources["sync_pairing"], "single cross-producer typed B/E authority")) {
				t.Fatalf("callstack provenance overclaimed or lost typed authorities: %+v", coverage.FieldSources)
			}
			if test.wantEmit == 2 && test.dur == 0 {
				begin := strings.Index(body, "tracing_mark_write: B|")
				end := strings.Index(body, "tracing_mark_write: E|")
				if begin < 0 || end <= begin {
					t.Fatalf("zero-duration endpoint order changed: %q", body)
				}
			}
		})
	}

	t.Run("global and lane poison reject exact points", func(t *testing.T) {
		for _, lifecycle := range []traceDBLifecycleIndex{
			{GlobalPoison: []int64{1000}},
			{ByTID: map[int64]traceDBLifecycleLane{101: {PoisonPoints: []int64{1000}}}},
			{ByPID: map[int64]traceDBLifecycleLane{100: {PoisonPoints: []int64{1000}}}},
		} {
			statements := traceDBCallstackAuthorityStatements(
				[]string{"INSERT INTO thread_state VALUES (1, 1000, 1, 1, 'Running')"},
				[]string{"INSERT INTO callstack VALUES (1, 1000, 0, 1, NULL, 'zero', '', NULL, NULL, 0)"},
			)
			coverage, _ := exportTraceDBCallstackAuthorityFixture(t, statements, lifecycle, true, nil)
			if coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "lifecycle_rejected_sync_point=1") {
				t.Fatalf("poison point admitted sync endpoint: %+v", coverage)
			}
		}
	})

	t.Run("global and lane poison reject positive closed ends", func(t *testing.T) {
		for _, lifecycle := range []traceDBLifecycleIndex{
			{GlobalPoison: []int64{2000}},
			{ByTID: map[int64]traceDBLifecycleLane{101: {PoisonPoints: []int64{2000}}}},
			{ByPID: map[int64]traceDBLifecycleLane{100: {PoisonPoints: []int64{2000}}}},
		} {
			statements := traceDBCallstackAuthorityStatements(
				[]string{"INSERT INTO thread_state VALUES (1, 900, 1101, 1, 'Running')"},
				[]string{"INSERT INTO callstack VALUES (1, 1000, 1000, 1, NULL, 'positive', '', NULL, NULL, 0)"},
			)
			coverage, _ := exportTraceDBCallstackAuthorityFixture(t, statements, lifecycle, true, nil)
			if coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "lifecycle_rejected_sync_closed_interval=1") {
				t.Fatalf("poison at positive closed end admitted span: %+v", coverage)
			}
		}
	})
}

func TestTraceDBCallstackSyncAntiRescueIsLaneLocalAndOrderIndependent(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			bad := "INSERT INTO callstack VALUES (1, 1000, 600, 1, NULL, 'bad-cross-cut', '', NULL, NULL, 0)"
			good := "INSERT INTO callstack VALUES (2, 1100, 100, 1, NULL, 'same-lane-good', '', NULL, NULL, 0)"
			if reverse {
				bad, good = good, bad
			}
			statements := traceDBCallstackAuthorityStatements(
				[]string{
					"INSERT INTO thread_state VALUES (1, 900, 500, 1, 'Running')",
					"INSERT INTO thread_state VALUES (3, 900, 1300, 3, 'Running')",
				},
				[]string{bad, good, "INSERT INTO callstack VALUES (3, 1200, 100, 3, NULL, 'other-lane-good', '', NULL, NULL, 0)"},
			)
			coverage, body := exportTraceDBCallstackAuthorityFixture(t, statements,
				traceDBCallstackCutLifecycle(1500, true, true, 2, 2), true, nil)
			if coverage.RowsEmitted != 2 || !strings.Contains(coverage.Skipped, "lifecycle_rejected_sync_closed_interval=1") ||
				!strings.Contains(coverage.Skipped, "sync_span_authority: suppressed_spans=1 suppressed_endpoints=2") ||
				strings.Contains(body, "same-lane-good") || !strings.Contains(body, "other-lane-good") {
				t.Fatalf("sync anti-rescue/locality mismatch: coverage=%+v body=%q", coverage, body)
			}
		})
	}
}

func TestTraceDBCallstackUnknownEndCPUPreservesLaminarSameLaneSpans(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			bad := "INSERT INTO callstack VALUES (1, 1000, 600, 1, NULL, 'missing-end-cpu', '', NULL, NULL, 0)"
			good := "INSERT INTO callstack VALUES (2, 1100, 100, 1, NULL, 'same-lane-good', '', NULL, NULL, 1)"
			if reverse {
				bad, good = good, bad
			}
			statements := traceDBCallstackAuthorityStatements(
				[]string{
					"INSERT INTO thread_state VALUES (1, 900, 500, 1, 'Running')",
					"INSERT INTO thread_state VALUES (3, 900, 1300, 3, 'Running')",
				},
				[]string{bad, good, "INSERT INTO callstack VALUES (3, 1200, 100, 3, NULL, 'other-lane-good', '', NULL, NULL, 0)"},
			)
			coverage, body := exportTraceDBCallstackAuthorityFixture(t, statements, traceDBLifecycleIndex{}, true, nil)
			if coverage.RowsEmitted != 6 || coverage.Skipped != "" ||
				coverage.Metrics["source_rows_preserved_cpu_unavailable"] != 1 ||
				!strings.Contains(body, "reason=unknown_end_cpu") ||
				!strings.Contains(body, "name=bWlzc2luZy1lbmQtY3B1") ||
				!strings.Contains(body, "same-lane-good") ||
				!strings.Contains(body, "other-lane-good") {
				t.Fatalf("unknown end CPU did not preserve proven laminar spans: coverage=%+v body=%q", coverage, body)
			}
		})
	}
}

func TestTraceDBCallstackMalformedSyncRowsPoisonExactLane(t *testing.T) {
	tests := []struct {
		name       string
		badRow     string
		wantReason string
		localOnly  bool
	}{
		{
			name:       "invalid duration storage",
			badRow:     "INSERT INTO callstack VALUES (1, 1000, CAST(100 AS TEXT), 1, NULL, 'bad-duration', '', NULL, NULL, 0)",
			wantReason: "invalid_duration=1",
		},
		{
			name:       "overflow interval",
			badRow:     "INSERT INTO callstack VALUES (1, 9223372036854775807, 1, 1, NULL, 'overflow', '', NULL, NULL, 0)",
			wantReason: "interval_overflow=1",
			localOnly:  true,
		},
		{
			name:       "invalid depth",
			badRow:     "INSERT INTO callstack VALUES (1, 1000, 100, 1, NULL, 'bad-depth', '', NULL, NULL, -1)",
			wantReason: "invalid_depth=1",
			localOnly:  true,
		},
		{
			name:       "unknown flag is also potential sync",
			badRow:     "INSERT INTO callstack VALUES (1, 1000, 0, 1, NULL, 'bad-flag', 's', 7, NULL, 0)",
			wantReason: "unknown_flag=1",
			localOnly:  true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statements := traceDBCallstackAuthorityStatements(
				[]string{
					"INSERT INTO thread_state VALUES (1, 900, 1300, 1, 'Running')",
					"INSERT INTO thread_state VALUES (3, 900, 1300, 3, 'Running')",
				},
				[]string{
					test.badRow,
					"INSERT INTO callstack VALUES (2, 1200, 100, 1, NULL, 'same-lane-good', '', NULL, NULL, 0)",
					"INSERT INTO callstack VALUES (3, 1400, 100, 3, NULL, 'other-lane-good', '', NULL, NULL, 0)",
				},
			)
			coverage, body := exportTraceDBCallstackAuthorityFixture(t, statements, traceDBLifecycleIndex{}, true, nil)
			wantRows := 2
			wantSameLane := false
			wantSuppression := true
			if test.localOnly {
				wantRows = 4
				wantSameLane = true
				wantSuppression = false
			}
			hasSameLane := strings.Contains(body, "same-lane-good")
			hasSuppression := strings.Contains(coverage.Skipped, "sync_span_authority: suppressed_spans=1 suppressed_endpoints=2")
			if coverage.RowsEmitted != wantRows || !strings.Contains(coverage.Skipped, test.wantReason) ||
				hasSuppression != wantSuppression || hasSameLane != wantSameLane ||
				!strings.Contains(body, "other-lane-good") ||
				(!test.localOnly && !strings.Contains(coverage.Skipped, "localized_fence_declarations=1")) {
				t.Fatalf("malformed sync anti-rescue mismatch: coverage=%+v body=%q", coverage, body)
			}
		})
	}
}

func TestTraceDBCallstackRejectedIntervalFencesOnlyOverlap(t *testing.T) {
	statements := traceDBCallstackAuthorityStatements(
		[]string{
			"INSERT INTO thread_state VALUES (1, 700, 1400, 1, 'Running')",
			"INSERT INTO thread_state VALUES (3, 700, 1400, 3, 'Running')",
		},
		[]string{
			"INSERT INTO callstack VALUES (1, 1000, 100, 1, NULL, 'bad-depth', '', NULL, NULL, -1)",
			"INSERT INTO callstack VALUES (2, 800, 100, 1, NULL, 'prefix-kept', '', NULL, NULL, 0)",
			"INSERT INTO callstack VALUES (3, 1050, 25, 1, NULL, 'overlap-suppressed', '', NULL, NULL, 0)",
			"INSERT INTO callstack VALUES (4, 1200, 100, 1, NULL, 'suffix-kept', '', NULL, NULL, 0)",
			"INSERT INTO callstack VALUES (5, 1200, 100, 3, NULL, 'other-lane-kept', '', NULL, NULL, 0)",
		},
	)
	coverage, body := exportTraceDBCallstackAuthorityFixture(t, statements, traceDBLifecycleIndex{}, true, nil)
	if coverage.RowsEmitted != 6 ||
		coverage.Metrics["sync_spans_suppressed_by_local_fence"] != 1 ||
		coverage.Metrics["localized_fence_witnesses_emitted"] != 1 ||
		coverage.Metadata["localized_fence_witnesses"] !=
			"tid=101/itid=1/kind=interval/start_ns=1000/end_ns=1100/reason=rejected_callstack_candidate" ||
		!strings.Contains(coverage.Skipped, "invalid_depth=1") ||
		!strings.Contains(coverage.Skipped, "localized_fence_declarations=1 suppressed_spans=1") ||
		!strings.Contains(body, "prefix-kept") ||
		!strings.Contains(body, "suffix-kept") ||
		!strings.Contains(body, "other-lane-kept") ||
		strings.Contains(body, "overlap-suppressed") {
		t.Fatalf("callstack overlap fence scope drifted: coverage=%+v body=%q", coverage, body)
	}
}

func TestTraceDBCallstackRejectedTimestampFencesOnlySuffix(t *testing.T) {
	statements := traceDBCallstackAuthorityStatements(
		[]string{
			"INSERT INTO thread_state VALUES (1, 700, 1400, 1, 'Running')",
			"INSERT INTO thread_state VALUES (3, 700, 1400, 3, 'Running')",
		},
		[]string{
			"INSERT INTO callstack VALUES (1, 1000, CAST(100 AS TEXT), 1, NULL, 'bad-duration', '', NULL, NULL, 0)",
			"INSERT INTO callstack VALUES (2, 800, 100, 1, NULL, 'prefix-kept', '', NULL, NULL, 0)",
			"INSERT INTO callstack VALUES (3, 1200, 100, 1, NULL, 'suffix-suppressed', '', NULL, NULL, 0)",
			"INSERT INTO callstack VALUES (4, 1200, 100, 3, NULL, 'other-lane-kept', '', NULL, NULL, 0)",
		},
	)
	coverage, body := exportTraceDBCallstackAuthorityFixture(t, statements, traceDBLifecycleIndex{}, true, nil)
	if coverage.RowsEmitted != 4 ||
		coverage.Metrics["sync_spans_suppressed_by_local_fence"] != 1 ||
		coverage.Metrics["localized_fence_witnesses_emitted"] != 1 ||
		coverage.Metrics["rejected_callstack_fence_witnesses_emitted"] != 1 ||
		coverage.Metadata["localized_fence_witnesses"] !=
			"tid=101/itid=1/kind=suffix/start_ns=1000/end_ns=0/reason=rejected_callstack_candidate" ||
		coverage.Metadata["rejected_callstack_fence_witnesses"] !=
			"row_id=1/tid=101/itid=1/ts_ns=1000/reason=invalid_duration/dur=text_bytes=3/b64=MTAw" ||
		!strings.Contains(coverage.Skipped, "invalid_duration=1") ||
		!strings.Contains(coverage.Skipped, "localized_fence_declarations=1 suppressed_spans=1") ||
		!strings.Contains(body, "prefix-kept") ||
		!strings.Contains(body, "other-lane-kept") ||
		strings.Contains(body, "suffix-suppressed") {
		t.Fatalf("callstack suffix fence scope drifted: coverage=%+v body=%q", coverage, body)
	}
}

func TestTraceDBCallstackNullDurationIsTypedAndRetainsExactClosureHint(t *testing.T) {
	statements := traceDBCallstackAuthorityStatements(
		[]string{
			"INSERT INTO thread_state VALUES (1, 700, 1400, 1, 'Running')",
		},
		[]string{
			"INSERT INTO callstack VALUES (1, 1000, NULL, 1, NULL, 'open-frame', '', NULL, NULL, 0)",
		},
	)
	coverage, body := exportTraceDBCallstackAuthorityFixture(
		t, statements, traceDBLifecycleIndex{}, true, nil)
	if body != "" ||
		coverage.Metrics["source_rows_rejected_invalid_duration_null"] != 1 ||
		coverage.Metrics["source_rows_rejected_null_duration_exact_hints_retained"] != 1 ||
		coverage.Metrics["source_rows_rejected_null_duration_exact_hint_unavailable"] != 0 ||
		!strings.Contains(coverage.Skipped, "invalid_duration=1") ||
		!strings.Contains(coverage.Metadata["rejected_callstack_fence_witnesses"],
			"reason=invalid_duration/dur=null") {
		t.Fatalf("NULL-duration typed hint drifted: coverage=%+v body=%q",
			coverage, body)
	}
}

func TestTraceDBCallstackRejectedFenceWitnessesAreBounded(t *testing.T) {
	callstack := make([]string, 0, traceDBCallstackRejectedWitnessCap+2)
	for index := 0; index < traceDBCallstackRejectedWitnessCap+2; index++ {
		callstack = append(callstack, fmt.Sprintf(
			"INSERT INTO callstack VALUES (%d, %d, CAST(%d AS TEXT), 1, NULL, 'bad-duration', '', NULL, NULL, 0)",
			index+1, 1000+index, index+10))
	}
	statements := traceDBCallstackAuthorityStatements(nil, callstack)
	coverage, body := exportTraceDBCallstackAuthorityFixture(
		t, statements, traceDBLifecycleIndex{}, true, nil)
	if body != "" ||
		coverage.Metrics["rejected_callstack_fence_witnesses_emitted"] !=
			traceDBCallstackRejectedWitnessCap ||
		coverage.Metrics["rejected_callstack_fence_witnesses_omitted"] != 2 ||
		strings.Count(coverage.Metadata["rejected_callstack_fence_witnesses"],
			"reason=invalid_duration") != traceDBCallstackRejectedWitnessCap ||
		!strings.Contains(coverage.Metadata["rejected_callstack_fence_witnesses"],
			"row_id=1/tid=101/itid=1/ts_ns=1000/reason=invalid_duration/dur=text_bytes=2/b64=MTA") {
		t.Fatalf("bounded rejected-row witnesses drifted: coverage=%+v body=%q",
			coverage, body)
	}
}

func TestTraceDBCallstackRejectedUnlocalizableTimePoisonsLane(t *testing.T) {
	statements := traceDBCallstackAuthorityStatements(
		[]string{
			"INSERT INTO thread_state VALUES (1, 700, 1400, 1, 'Running')",
			"INSERT INTO thread_state VALUES (3, 700, 1400, 3, 'Running')",
		},
		[]string{
			"INSERT INTO callstack VALUES (1, CAST(1000 AS TEXT), 100, 1, NULL, 'bad-time', '', NULL, NULL, 0)",
			"INSERT INTO callstack VALUES (2, 800, 100, 1, NULL, 'same-lane-prefix', '', NULL, NULL, 0)",
			"INSERT INTO callstack VALUES (3, 1200, 100, 1, NULL, 'same-lane-suffix', '', NULL, NULL, 0)",
			"INSERT INTO callstack VALUES (4, 1200, 100, 3, NULL, 'other-lane-kept', '', NULL, NULL, 0)",
		},
	)
	coverage, body := exportTraceDBCallstackAuthorityFixture(t, statements, traceDBLifecycleIndex{}, true, nil)
	if coverage.RowsEmitted != 2 ||
		!strings.Contains(coverage.Skipped, "invalid_timestamp=1") ||
		!strings.Contains(coverage.Skipped, "exact_lane_poison_declarations=1") ||
		!strings.Contains(coverage.Skipped, "suppressed_spans=2 suppressed_endpoints=4") ||
		strings.Contains(body, "same-lane-prefix") ||
		strings.Contains(body, "same-lane-suffix") ||
		!strings.Contains(body, "other-lane-kept") {
		t.Fatalf("callstack unlocalizable-time fail-closed scope drifted: coverage=%+v body=%q", coverage, body)
	}
}

func TestTraceDBCallstackSyncBarrierUsesOnlyExactCandidateLanes(t *testing.T) {
	t.Run("dual claim mismatch taints both exact lanes", func(t *testing.T) {
		statements := traceDBCallstackAuthorityStatements(
			[]string{
				"INSERT INTO thread_state VALUES (1, 900, 1300, 1, 'Running')",
				"INSERT INTO thread_state VALUES (2, 900, 1300, 2, 'Running')",
				"INSERT INTO thread_state VALUES (3, 900, 1300, 3, 'Running')",
			},
			[]string{
				"INSERT INTO callstack VALUES (1, 1000, 100, 1, 2, 'identity-conflict', '', NULL, NULL, 0)",
				"INSERT INTO callstack VALUES (2, 1050, 25, 1, NULL, 'lane-one-overlap', '', NULL, NULL, 0)",
				"INSERT INTO callstack VALUES (3, 1050, 25, 2, NULL, 'lane-two-overlap', '', NULL, NULL, 0)",
				"INSERT INTO callstack VALUES (4, 1200, 100, 1, NULL, 'lane-one-later', '', NULL, NULL, 0)",
				"INSERT INTO callstack VALUES (5, 1400, 100, 2, NULL, 'lane-two-later', '', NULL, NULL, 0)",
				"INSERT INTO callstack VALUES (6, 1600, 100, 3, NULL, 'lane-three', '', NULL, NULL, 0)",
			},
		)
		coverage, body := exportTraceDBCallstackAuthorityFixture(t, statements, traceDBLifecycleIndex{}, true, nil)
		if coverage.RowsEmitted != 6 || !strings.Contains(coverage.Skipped, "emitter_identity_mismatch=1") ||
			!strings.Contains(coverage.Skipped, "localized_fence_declarations=2 suppressed_spans=2") ||
			strings.Contains(body, "lane-one-overlap") || strings.Contains(body, "lane-two-overlap") ||
			!strings.Contains(body, "lane-one-later") || !strings.Contains(body, "lane-two-later") ||
			!strings.Contains(body, "lane-three") {
			t.Fatalf("dual exact candidate barrier mismatch: coverage=%+v body=%q", coverage, body)
		}
	})

	t.Run("untyped identity noise does not globalize", func(t *testing.T) {
		statements := traceDBCallstackAuthorityStatements(
			[]string{"INSERT INTO thread_state VALUES (3, 900, 1300, 3, 'Running')"},
			[]string{
				"INSERT INTO callstack VALUES (1, 1000, 100, CAST(3 AS TEXT), NULL, 'noise', '', NULL, NULL, 0)",
				"INSERT INTO callstack VALUES (2, 1200, 100, 3, NULL, 'exact', '', NULL, NULL, 0)",
			},
		)
		coverage, body := exportTraceDBCallstackAuthorityFixture(t, statements, traceDBLifecycleIndex{}, true, nil)
		if coverage.RowsEmitted != 2 || !strings.Contains(coverage.Skipped, "invalid_emitter_itid=1") || !strings.Contains(body, "exact") {
			t.Fatalf("untyped identity noise affected exact lane: coverage=%+v body=%q", coverage, body)
		}
	})

	for _, test := range []struct {
		name string
		row  string
	}{
		{name: "numeric unresolved itid", row: "INSERT INTO callstack VALUES (1, 1000, 100, 999, NULL, 'noise', '', NULL, NULL, 0)"},
		{name: "numeric unresolved callid", row: "INSERT INTO callstack VALUES (1, 1000, 100, NULL, 999, 'noise', '', NULL, NULL, 0)"},
		{name: "missing both identities", row: "INSERT INTO callstack VALUES (1, 1000, 100, NULL, NULL, 'noise', '', NULL, NULL, 0)"},
	} {
		t.Run(test.name+" does not globalize", func(t *testing.T) {
			statements := traceDBCallstackAuthorityStatements(
				[]string{"INSERT INTO thread_state VALUES (3, 900, 1300, 3, 'Running')"},
				[]string{
					test.row,
					"INSERT INTO callstack VALUES (2, 1200, 100, 3, NULL, 'exact', '', NULL, NULL, 0)",
				},
			)
			coverage, body := exportTraceDBCallstackAuthorityFixture(t, statements, traceDBLifecycleIndex{}, true, nil)
			if coverage.RowsEmitted != 2 || !strings.Contains(body, "exact") ||
				strings.Contains(coverage.Skipped, "sync_span_authority: suppressed_spans") {
				t.Fatalf("unresolved identity noise affected an exact lane: coverage=%+v body=%q", coverage, body)
			}
		})
	}
}

func TestTraceDBCallstackAsyncEndpointsAllowThreadMigrationButRequireProcessContinuity(t *testing.T) {
	baseRows := []string{
		"INSERT INTO thread_state VALUES (1, 900, 200, 1, 'Running')",
		"INSERT INTO thread_state VALUES (4, 1900, 200, 4, 'Running')",
	}
	callRows := []string{
		"INSERT INTO callstack VALUES (1, 1000, 0, 1, NULL, 'async', 'S', 9, NULL, 0)",
		"INSERT INTO callstack VALUES (2, 2000, 0, 4, NULL, 'async', 'C', 9, NULL, 0)",
	}
	t.Run("cross-thread same-process", func(t *testing.T) {
		coverage, body := exportTraceDBCallstackAuthorityFixture(t,
			traceDBCallstackAuthorityStatements(baseRows, callRows), traceDBLifecycleIndex{}, true, nil)
		if coverage.RowsEmitted != 2 || !strings.Contains(body, "tracing_mark_write: S|100|async|9") ||
			!strings.Contains(body, "tracing_mark_write: F|100|async|9") {
			t.Fatalf("legal async migration rejected: coverage=%+v body=%q", coverage, body)
		}
	})

	t.Run("thread generation cut between endpoints does not block migration", func(t *testing.T) {
		lifecycle := traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
			101: {Cuts: []traceDBLifecycleBoundary{{TS: 1500, NewITID: 2, NewIPID: 2}}},
		}}
		coverage, body := exportTraceDBCallstackAuthorityFixture(t,
			traceDBCallstackAuthorityStatements(baseRows, callRows), lifecycle, true, nil)
		if coverage.RowsEmitted != 2 || !strings.Contains(body, "tracing_mark_write: S|100|async|9") ||
			!strings.Contains(body, "tracing_mark_write: F|100|async|9") {
			t.Fatalf("thread-only generation cut blocked legal async migration: coverage=%+v body=%q", coverage, body)
		}
	})

	t.Run("process cut away and back", func(t *testing.T) {
		lifecycle := traceDBLifecycleIndex{ByPID: map[int64]traceDBLifecycleLane{
			100: {Cuts: []traceDBLifecycleBoundary{
				{TS: 1500, NewIPID: 2, NewITID: 2},
				{TS: 1800, NewIPID: 1, NewITID: 1},
			}},
		}}
		coverage, body := exportTraceDBCallstackAuthorityFixture(t,
			traceDBCallstackAuthorityStatements(baseRows, callRows), lifecycle, true, nil)
		if coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "lifecycle_rejected_async_process_interval=2") || body != "" {
			t.Fatalf("async task crossed a process generation: coverage=%+v body=%q", coverage, body)
		}
	})
}

func TestTraceDBCallstackExactAsyncFailuresAreKeyLocal(t *testing.T) {
	tests := []struct {
		name                   string
		running                []string
		lifecycle              traceDBLifecycleIndex
		mutate                 func(*traceDBRunningIntegrity)
		wantReason             string
		preserveCPUUnavailable bool
	}{
		{
			name: "endpoint lifecycle",
			running: []string{
				"INSERT INTO thread_state VALUES (1, 900, 200, 1, 'Running')",
				"INSERT INTO thread_state VALUES (4, 1900, 200, 4, 'Running')",
			},
			lifecycle:  traceDBCallstackCutLifecycle(1000, true, false, 2, 2),
			wantReason: "lifecycle_rejected_async_endpoint=1",
		},
		{
			name: "finish endpoint lifecycle",
			running: []string{
				"INSERT INTO thread_state VALUES (1, 900, 200, 1, 'Running')",
				"INSERT INTO thread_state VALUES (4, 1900, 200, 4, 'Running')",
			},
			lifecycle:  traceDBCallstackCutLifecycle(2000, false, true, 2, 2),
			wantReason: "lifecycle_rejected_async_endpoint=1",
		},
		{
			name: "Running source taint",
			running: []string{
				"INSERT INTO thread_state VALUES (1, 900, 200, 1, 'Running')",
				"INSERT INTO thread_state VALUES (4, 1900, 200, 4, 'Running')",
			},
			mutate:                 func(integrity *traceDBRunningIntegrity) { integrity.TaintedITIDs[1] = true },
			wantReason:             "tainted_running_cpu_witness=1",
			preserveCPUUnavailable: true,
		},
		{
			name: "Running lifecycle rejection",
			running: []string{
				"INSERT INTO thread_state VALUES (1, 900, 900, 1, 'Running')",
				"INSERT INTO thread_state VALUES (4, 1900, 200, 4, 'Running')",
			},
			lifecycle:              traceDBCallstackCutLifecycle(1500, true, false, 2, 2),
			wantReason:             "lifecycle_rejected_running_cpu_witness=1",
			preserveCPUUnavailable: true,
		},
		{
			name: "unknown CPU",
			running: []string{
				"INSERT INTO thread_state VALUES (4, 1900, 200, 4, 'Running')",
			},
			wantReason:             "unknown_start_cpu=1",
			preserveCPUUnavailable: true,
		},
	}
	for _, test := range tests {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", test.name, reverse), func(t *testing.T) {
				running := append([]string(nil), test.running...)
				running = append(running, "INSERT INTO thread_state VALUES (3, 1100, 300, 3, 'Running')")
				calls := []string{
					"INSERT INTO callstack VALUES (1, 1000, 0, 1, NULL, 'bad-async', 'S', 9, NULL, 0)",
					"INSERT INTO callstack VALUES (2, 2000, 0, 4, NULL, 'bad-async', 'C', 9, NULL, 0)",
					"INSERT INTO callstack VALUES (3, 1200, 0, 3, NULL, 'good-async', 'S', 77, NULL, 0)",
					"INSERT INTO callstack VALUES (4, 1300, 0, 3, NULL, 'good-async', 'C', 77, NULL, 0)",
				}
				if reverse {
					for left, right := 0, len(calls)-1; left < right; left, right = left+1, right-1 {
						calls[left], calls[right] = calls[right], calls[left]
					}
				}
				coverage, body := exportTraceDBCallstackAuthorityFixture(t,
					traceDBCallstackAuthorityStatements(running, calls), test.lifecycle, true, test.mutate)
				if test.preserveCPUUnavailable {
					wireReason := strings.TrimSuffix(test.wantReason, "=1")
					if coverage.RowsEmitted != 4 || coverage.Skipped != "" ||
						coverage.Metrics["source_rows_preserved_cpu_unavailable"] != 1 ||
						!strings.Contains(body, "bad-async") || !strings.Contains(body, "good-async") ||
						!strings.Contains(body, "reason="+wireReason) {
						t.Fatalf("CPU-unavailable async endpoint was not preserved locally: coverage=%+v body=%q", coverage, body)
					}
					return
				}
				if coverage.RowsEmitted != 2 || !strings.Contains(coverage.Skipped, test.wantReason) ||
					!strings.Contains(coverage.Skipped, "async_key_fail_closed=1") || strings.Contains(body, "bad-async") ||
					!strings.Contains(body, "good-async") || strings.Contains(coverage.Skipped, "async_family_fail_closed") {
					t.Fatalf("exact async failure crossed key boundary: coverage=%+v body=%q", coverage, body)
				}
			})
		}
	}
}

func TestTraceDBCallstackAsyncEndpointPointAdmission(t *testing.T) {
	baseRows := []string{
		"INSERT INTO thread_state VALUES (1, 900, 200, 1, 'Running')",
		"INSERT INTO thread_state VALUES (4, 1900, 200, 4, 'Running')",
	}
	callRows := []string{
		"INSERT INTO callstack VALUES (1, 1000, 0, 1, NULL, 'async', 'S', 9, NULL, 0)",
		"INSERT INTO callstack VALUES (2, 2000, 0, 4, NULL, 'async', 'C', 9, NULL, 0)",
	}
	tests := []struct {
		name       string
		lifecycle  traceDBLifecycleIndex
		complete   bool
		wantReason string
		wantFamily string
	}{
		{
			name:       "start old identity at thread cut",
			lifecycle:  traceDBCallstackCutLifecycle(1000, true, false, 2, 2),
			complete:   true,
			wantReason: "lifecycle_rejected_async_endpoint=1",
			wantFamily: "async_key_fail_closed=1",
		},
		{
			name:       "finish old process at cut",
			lifecycle:  traceDBCallstackCutLifecycle(2000, false, true, 2, 2),
			complete:   true,
			wantReason: "lifecycle_rejected_async_endpoint=1",
			wantFamily: "async_key_fail_closed=1",
		},
		{
			name:       "incomplete authority",
			wantReason: "lifecycle_rejected_async_endpoint=2",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coverage, body := exportTraceDBCallstackAuthorityFixture(t,
				traceDBCallstackAuthorityStatements(baseRows, callRows), test.lifecycle, test.complete, nil)
			if coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, test.wantReason) ||
				(test.wantFamily != "" && !strings.Contains(coverage.Skipped, test.wantFamily)) || body != "" {
				t.Fatalf("async endpoint point gate mismatch: coverage=%+v body=%q", coverage, body)
			}
		})
	}

	t.Run("same-time endpoints remain ordered start then finish", func(t *testing.T) {
		coverage, body := exportTraceDBCallstackAuthorityFixture(t,
			traceDBCallstackAuthorityStatements(
				[]string{
					"INSERT INTO thread_state VALUES (1, 1000, 1, 1, 'Running')",
					"INSERT INTO thread_state VALUES (4, 1000, 1, 4, 'Running')",
				},
				[]string{
					"INSERT INTO callstack VALUES (1, 1000, 0, 1, NULL, 'async', 'S', 9, NULL, 0)",
					"INSERT INTO callstack VALUES (2, 1000, 0, 4, NULL, 'async', 'C', 9, NULL, 0)",
				},
			), traceDBLifecycleIndex{}, true, nil)
		if coverage.RowsEmitted != 2 || strings.Index(body, "tracing_mark_write: S|") < 0 ||
			strings.Index(body, "tracing_mark_write: F|") <= strings.Index(body, "tracing_mark_write: S|") {
			t.Fatalf("same-time async order changed: coverage=%+v body=%q", coverage, body)
		}
	})
}

func TestTraceDBCallstackTypedRunningStatusesRemainDistinct(t *testing.T) {
	tests := []struct {
		name       string
		running    []string
		lifecycle  traceDBLifecycleIndex
		mutate     func(*traceDBRunningIntegrity)
		wantReason string
	}{
		{
			name:       "source tainted",
			running:    []string{"INSERT INTO thread_state VALUES (1, 900, 1300, 1, 'Running')"},
			mutate:     func(integrity *traceDBRunningIntegrity) { integrity.TaintedITIDs[1] = true },
			wantReason: "tainted_running_cpu_witness=1",
		},
		{
			name:       "running lifecycle rejected",
			running:    []string{"INSERT INTO thread_state VALUES (2, 1400, 500, 2, 'Running')"},
			lifecycle:  traceDBCallstackCutLifecycle(1500, true, true, 2, 2),
			wantReason: "lifecycle_rejected_running_cpu_witness=1",
		},
		{
			name:       "unknown CPU",
			wantReason: "unknown_start_cpu=1",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			itid, ts := int64(1), int64(1000)
			if test.name == "running lifecycle rejected" {
				itid, ts = 2, 1600
			}
			statements := traceDBCallstackAuthorityStatements(test.running,
				[]string{fmt.Sprintf("INSERT INTO callstack VALUES (1, %d, 0, %d, NULL, 'point|exact', '', NULL, NULL, 0)", ts, itid)})
			coverage, body := exportTraceDBCallstackAuthorityFixture(t, statements, test.lifecycle, true, test.mutate)
			if coverage.RowsEmitted != 2 || coverage.Skipped != "" ||
				coverage.Metrics["source_rows_preserved_cpu_unavailable"] != 1 ||
				coverage.Metrics["source_rows_admitted_exact_name_pre_pairing"] != 1 ||
				!strings.Contains(body, "reason="+strings.TrimSuffix(test.wantReason, "=1")) {
				t.Fatalf("typed Running status collapsed: want=%q coverage=%+v", test.wantReason, coverage)
			}
		})
	}
}

func TestTraceDBCallstackSamePublicTIDSchedulerAliasPreservesNamespacePID(t *testing.T) {
	base := []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 17267, 'host-process')",
		"INSERT INTO process VALUES (2, 37722, 'namespace-process')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 17267, 1, '.ugc.aweme.lite', 0, 1, 10)",
		"INSERT INTO thread VALUES (2, 17267, 2, '.ugc.aweme.lite', 0, 0, 0)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (1, 900, 1201, 3, 'Running')",
		"CREATE TABLE callstack (id, ts, dur, itid, callid, name, flag, cookie, chainId, depth)",
		"INSERT INTO callstack VALUES (1, 1000, 1000, 2, NULL, 'Choreographer|doFrame', '', NULL, NULL, 0)",
	}
	coverage, body := exportTraceDBCallstackAuthorityFixture(t, base, traceDBLifecycleIndex{}, true, nil)
	if coverage.RowsEmitted != 2 ||
		coverage.Metrics["source_rows_recovered_same_public_tid_scheduler_alias"] != 1 ||
		coverage.Metrics["source_rows_admitted_exact_name_pre_pairing"] != 1 ||
		coverage.Metrics["standard_sync_pipe_spans_emitted"] != 1 ||
		!strings.Contains(body, "-17267 (17267)") ||
		!strings.Contains(body, "[003]") ||
		!strings.Contains(body, "tracing_mark_write: B|37722|Choreographer|doFrame") ||
		strings.Contains(body, "# codrax_trace_mark_exact/v1") {
		t.Fatalf("host/namespace PID alias lost physical evidence: coverage=%+v body=%q", coverage, body)
	}

	ambiguous := append([]string(nil), base[:len(base)-3]...)
	ambiguous = append(ambiguous,
		"INSERT INTO process VALUES (3, 47722, 'second-host')",
		"INSERT INTO thread VALUES (3, 17267, 3, '.ugc.aweme.lite', 0, 0, 10)",
		"INSERT INTO thread_state VALUES (1, 900, 1201, 3, 'Running')",
		"INSERT INTO thread_state VALUES (3, 900, 1201, 4, 'Running')",
		"CREATE TABLE callstack (id, ts, dur, itid, callid, name, flag, cookie, chainId, depth)",
		"INSERT INTO callstack VALUES (1, 1000, 1000, 2, NULL, 'ambiguous', '', NULL, NULL, 0)",
	)
	coverage, body = exportTraceDBCallstackAuthorityFixture(t, ambiguous, traceDBLifecycleIndex{}, true, nil)
	if coverage.RowsEmitted != 2 || coverage.Skipped != "" ||
		coverage.Metrics["source_rows_preserved_cpu_unavailable"] != 1 ||
		!strings.Contains(body, "span_pid=37722") ||
		!strings.Contains(body, "reason=ambiguous_same_public_tid_scheduler_alias") {
		t.Fatalf("ambiguous host scheduler aliases fabricated CPU placement or lost the span: coverage=%+v body=%q", coverage, body)
	}
}

func TestTraceDBCallstackCompletedAsyncIntervalPreservesUnavailableCPUAndNamespacePID(t *testing.T) {
	t.Run("CPU unavailable remains typed", func(t *testing.T) {
		coverage, body := exportTraceDBCallstackAuthorityFixture(t,
			traceDBCallstackAuthorityStatements(nil, []string{
				"INSERT INTO callstack VALUES (1, 1000, 500, 1, NULL, 'official-async', NULL, 9, NULL, 0)",
			}), traceDBLifecycleIndex{}, true, nil)
		if coverage.RowsEmitted != 1 ||
			coverage.Metrics["source_rows_emitted_official_async_interval"] != 1 ||
			coverage.Metrics["source_rows_preserved_cpu_unavailable"] != 1 ||
			!strings.Contains(body, "# codrax_trace_async_interval/v1") ||
			!strings.Contains(body, "start_cpu=~") ||
			!strings.Contains(body, "cpu_reason=unknown_start_cpu") {
			t.Fatalf("completed async unavailable-CPU authority drifted: coverage=%+v body=%q", coverage, body)
		}
	})

	t.Run("host header and namespace marker pid remain separate", func(t *testing.T) {
		statements := []string{
			"CREATE TABLE trace_range (start_ts)",
			"INSERT INTO trace_range VALUES (0)",
			"CREATE TABLE process (ipid, pid, name)",
			"INSERT INTO process VALUES (1, 17267, 'host-process')",
			"INSERT INTO process VALUES (2, 37722, 'namespace-process')",
			"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
			"INSERT INTO thread VALUES (1, 17267, 1, '.ugc.aweme.lite', 0, 1, 10)",
			"INSERT INTO thread VALUES (2, 17267, 2, '.ugc.aweme.lite', 0, 0, 0)",
			"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
			"INSERT INTO thread_state VALUES (1, 900, 1201, 3, 'Running')",
			"CREATE TABLE callstack (id, ts, dur, itid, callid, name, flag, cookie, chainId, depth)",
			"INSERT INTO callstack VALUES (1, 1000, 1000, 2, NULL, 'official-async', NULL, 9, NULL, 0)",
		}
		coverage, body := exportTraceDBCallstackAuthorityFixture(t, statements, traceDBLifecycleIndex{}, true, nil)
		path := filepath.Join(t.TempDir(), "namespace-async.systrace")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		idx, err := tracequery.BuildIndex(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		var event tracequery.Event
		if len(idx.Events) == 1 {
			event = idx.Events[0]
		}
		if coverage.RowsEmitted != 1 ||
			coverage.Metrics["source_rows_recovered_same_public_tid_scheduler_alias"] != 1 ||
			len(idx.Events) != 1 || event.Type != tracequery.EventTraceAsyncInterval ||
			event.PID != 17267 || event.TGID != 17267 || event.SpanPID != 37722 || event.CPU != 3 {
			t.Fatalf("completed async namespace authority drifted: coverage=%+v event=%+v body=%q",
				coverage, event, body)
		}
	})
}

func TestTraceDBCallstackThreadRenameIsDisplayOnly(t *testing.T) {
	for _, name := range []string{"old-thread", "renamed-thread", "bad\nname"} {
		t.Run(strings.ReplaceAll(name, "\n", "_newline_"), func(t *testing.T) {
			statements := traceDBCallstackAuthorityStatements(
				[]string{"INSERT INTO thread_state VALUES (1, 900, 1300, 1, 'Running')"},
				[]string{"INSERT INTO callstack VALUES (1, 1000, 100, 1, NULL, 'sync', '', NULL, NULL, 0)"},
			)
			for i, statement := range statements {
				if strings.HasPrefix(statement, "INSERT INTO thread VALUES (1, 101, 1,") {
					statements[i] = fmt.Sprintf("INSERT INTO thread VALUES (1, 101, 1, '%s', 0, 0, 1)", name)
				}
			}
			coverage, body := exportTraceDBCallstackAuthorityFixture(t, statements, traceDBLifecycleIndex{}, true, nil)
			if coverage.RowsEmitted != 2 || coverage.Skipped != "" || !strings.Contains(body, "[001]") ||
				!strings.Contains(body, "B|100|sync") {
				t.Fatalf("display rename changed hard callstack evidence: coverage=%+v body=%q", coverage, body)
			}
		})
	}
}

func TestTraceDBCallstackExactCandidateOrderIsDeterministic(t *testing.T) {
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 100}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 101, IPID: 1}
	index.ByITID[2] = traceDBThread{ITID: 2, TID: 102, IPID: 1}
	index.ThreadIDToITID[2] = 2
	authority := traceDBSchedulerAuthority{identities: index, initialized: true, complete: true}
	got := traceDBCallstackExactEmitterCandidates(authority, true, true, int64(1), int64(2))
	if fmt.Sprint(got) != "[1 2]" || !sort.SliceIsSorted(got, func(i, j int) bool { return got[i] < got[j] }) {
		t.Fatalf("exact barrier candidates=%v", got)
	}
}
