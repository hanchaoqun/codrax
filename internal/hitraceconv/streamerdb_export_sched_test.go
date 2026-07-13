package hitraceconv

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
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
		"INSERT INTO sched_slice VALUES (1200000, 100000, 1, 'R', 20, 3)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT, value REAL)",
		"INSERT INTO instant VALUES (900000, 'sched_wakeup', 2, 3, 'itid', NULL)",
		"CREATE TABLE raw (ts INT, name TEXT, cpu INT, itid INT)",
		"INSERT INTO raw VALUES (900000, 'sched_wakeup', 7, 2)",
		"CREATE TABLE data_dict (id INT, data TEXT)",
		"INSERT INTO data_dict VALUES (1, 'irq')",
		"INSERT INTO data_dict VALUES (2, 'irq_ret')",
		"INSERT INTO data_dict VALUES (3, 'handled')",
		"INSERT INTO data_dict VALUES (4, 'vec')",
		"INSERT INTO data_dict VALUES (5, 'RCU')",
		"CREATE TABLE args (argset INT, key INT, datatype INT, value INT)",
		"INSERT INTO args VALUES (10, 1, 0, 32)",
		"INSERT INTO args VALUES (10, 2, 1, 3)",
		"INSERT INTO args VALUES (20, 4, 0, 9)",
		"INSERT INTO args VALUES (20, 2, 1, 5)",
		"CREATE TABLE irq (ts INT, dur INT, callid INT, cat TEXT, name TEXT, argsetid INT)",
		"INSERT INTO irq VALUES (1500000, 10000, 4, 'irq', 'uart', 10)",
		"INSERT INTO irq VALUES (1600000, 20000, 6, 'softirq', 'RCU', 20)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (3, 800000, 200000, 4, 'Running')",
		"CREATE TABLE callstack (callid INT, ts INT)",
		"CREATE TABLE syscall (itid INT, ts INT)",
		"CREATE TABLE native_hook (itid INT, start_ts INT)",
		"CREATE TABLE frame_slice (itid INT, ts INT)",
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
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, _, err := exportTraceDBSchedulerFamilies(context.Background(), tdb, sink, syncSpans)
	if err != nil {
		t.Fatalf("export scheduler families: %v", err)
	}
	coverage, _, _ = finalizeTraceDBTestSyncSpans(t, sink, syncSpans, coverage)
	assertCoverageEmitted(t, coverage, "metadata", "thread", 9)
	assertCoverageEmitted(t, coverage, "scheduler", "sched_slice", 1)
	assertCoverageEmitted(t, coverage, "scheduler", "instant", 1)
	assertCoverageEmitted(t, coverage, "irq", "irq", 4)

	outPath := filepath.Join(t.TempDir(), "out.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	stats, writeErr := sink.prepareAndWriteForTest(context.Background(), out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatalf("write sorted rows: %v", writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if stats.RowsWritten != 15 || stats.SpillChunks == 0 {
		t.Fatalf("unexpected row sink stats: %+v", stats)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{
		"task_rename: pid=201 oldcomm=WorkerThread newcomm=WorkerThread",
		"sched_wakeup: comm=WorkerThread pid=201 prio=42 target_cpu=007",
		"[004] ....",
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
	if strings.Count(body, "sched_switch:") != 1 || strings.Contains(body, "next_comm=swapper") {
		t.Fatalf("scheduler export must contain only the one real adjacent boundary:\n%s", body)
	}
	idx, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery parse DB scheduler output: %v", err)
	}
	if len(idx.Events) < 7 {
		t.Fatalf("tracequery should parse scheduler/irq rows, got %+v", idx.Events)
	}
	var wakeups, switches []tracequery.Event
	for _, ev := range idx.Events {
		if ev.Type == tracequery.EventSchedWakeup {
			wakeups = append(wakeups, ev)
		}
		if ev.Type == tracequery.EventSchedSwitch {
			switches = append(switches, ev)
		}
	}
	if len(wakeups) != 1 || wakeups[0].WakeePID != 201 || wakeups[0].CPU != 4 || wakeups[0].TargetCPU != 7 ||
		wakeups[0].WakeePrioritySource() != "inferred_next_sched_slice" {
		t.Fatalf("wakeup metadata lost after round trip: %+v", wakeups)
	}
	if len(switches) != 1 || switches[0].Ts != 0.0012 || switches[0].CPU != 1 ||
		switches[0].PrevPID != 201 || switches[0].NextPID != 301 || switches[0].PrevState != "S" {
		t.Fatalf("real scheduler boundary lost after tracequery round trip: %+v", switches)
	}
	for _, item := range coverage {
		if item.Family == "scheduler" && item.Table == "sched_slice" {
			if item.RowsRead != 2 || item.RowsEmitted != 1 || item.PeakBuffered != 1 ||
				!strings.Contains(item.Skipped, "open_tail_rows=1") ||
				item.FieldSources["boundary_timestamp"] != "prev_sched_slice.ts+dur; requires exact equality with next_sched_slice.ts" ||
				item.FieldSources["open_tail"] != "final sched_slice is retained as an unclosed tail; no synthetic idle close" {
				t.Fatalf("sched_slice continuity coverage mismatch: %+v", item)
			}
		}
		if item.Family == "scheduler" && item.Table == "instant" {
			if item.FieldSources["header_cpu"] != "thread_state.Running.cpu" ||
				item.FieldSources["target_cpu"] != "raw.cpu" ||
				!strings.Contains(item.FieldSources["priority"], "inference") ||
				!strings.Contains(item.FieldSources["priority"], "non-exact") {
				t.Fatalf("wakeup field provenance missing: %+v", item)
			}
		}
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
	coverage, _, err := exportTraceDBSchedulerFamilies(context.Background(), tdb, sink, newTraceDBTestSyncSpanAuthority(t))
	if err != nil {
		t.Fatalf("missing tables should be coverage, not hard failure: %v", err)
	}
	if !coverageHasSkipped(coverage, "scheduler", "sched_slice", "missing table") ||
		!coverageHasSkipped(coverage, "scheduler", "instant", "missing table") ||
		!coverageHasSkipped(coverage, "irq", "irq", "missing table") {
		t.Fatalf("missing table coverage not recorded: %+v", coverage)
	}
}

func TestExportTraceDBSchedSwitchContinuityAudit(t *testing.T) {
	maxInt64 := strconv.FormatInt(math.MaxInt64, 10)
	tests := []struct {
		name        string
		rows        []string
		wantRead    int
		wantEmitted int
		wantSkipped []string
	}{
		{
			name: "exact",
			rows: []string{
				"INSERT INTO sched_slice VALUES (100, 100, 1, 'S', 40, 1)",
				"INSERT INTO sched_slice VALUES (200, 100, 1, 'R', 41, 2)",
			},
			wantRead: 2, wantEmitted: 1,
			wantSkipped: []string{"open_tail_rows=1", "synthetic_idle_closes=0"},
		},
		{
			name: "gap suppresses whole CPU lane",
			rows: []string{
				"INSERT INTO sched_slice VALUES (100, 50, 1, 'S', 40, 1)",
				"INSERT INTO sched_slice VALUES (200, 100, 1, 'R', 41, 2)",
			},
			wantRead: 2, wantEmitted: 0,
			wantSkipped: []string{"rows_suppressed=2", "cpu=001 suppressed_rows=2", "sched_slice_gap=1"},
		},
		{
			name: "overlap suppresses whole CPU lane",
			rows: []string{
				"INSERT INTO sched_slice VALUES (100, 150, 1, 'S', 40, 1)",
				"INSERT INTO sched_slice VALUES (200, 100, 1, 'R', 41, 2)",
			},
			wantRead: 2, wantEmitted: 0,
			wantSkipped: []string{"rows_suppressed=2", "cpu=001 suppressed_rows=2", "sched_slice_overlap=1"},
		},
		{
			name: "final null duration is open tail",
			rows: []string{
				"INSERT INTO sched_slice VALUES (100, 100, 1, 'S', 40, 1)",
				"INSERT INTO sched_slice VALUES (200, NULL, 1, 'R', 41, 2)",
			},
			wantRead: 2, wantEmitted: 1,
			wantSkipped: []string{"open_tail_rows=1", "synthetic_idle_closes=0"},
		},
		{
			name: "midstream null duration suppresses whole CPU lane",
			rows: []string{
				"INSERT INTO sched_slice VALUES (100, 100, 1, 'S', 40, 1)",
				"INSERT INTO sched_slice VALUES (200, NULL, 1, 'R', 41, 2)",
				"INSERT INTO sched_slice VALUES (300, 100, 1, 'R', 42, 1)",
			},
			wantRead: 3, wantEmitted: 0,
			wantSkipped: []string{"rows_suppressed=3", "cpu=001 suppressed_rows=3", "midstream_null_duration=1"},
		},
		{
			name: "end overflow suppresses whole CPU lane",
			rows: []string{
				"INSERT INTO sched_slice VALUES (" + strconv.FormatInt(math.MaxInt64-5, 10) + ", 10, 1, 'S', 40, 1)",
				"INSERT INTO sched_slice VALUES (" + maxInt64 + ", 0, 1, 'R', 41, 2)",
			},
			wantRead: 2, wantEmitted: 0,
			wantSkipped: []string{"rows_suppressed=2", "cpu=001 suppressed_rows=2", "sched_slice_end_overflow=1"},
		},
		{
			name: "integer invalid CPU does not poison valid CPU",
			rows: []string{
				"INSERT INTO sched_slice VALUES (100, 100, 1, 'S', 40, 1)",
				"INSERT INTO sched_slice VALUES (200, 100, 1, 'R', 41, 2)",
				"INSERT INTO sched_slice VALUES (300, 100, 4096, 'R', 42, 1)",
			},
			wantRead: 3, wantEmitted: 1,
			wantSkipped: []string{"rows_suppressed=1", "invalid_cpu_rows=1", "values=[4096]", "open_tail_rows=1"},
		},
		{
			name: "missing thread suppresses whole CPU lane",
			rows: []string{
				"INSERT INTO sched_slice VALUES (100, 100, 1, 'S', 40, 1)",
				"INSERT INTO sched_slice VALUES (200, 100, 1, 'R', 41, 999)",
			},
			wantRead: 2, wantEmitted: 0,
			wantSkipped: []string{"rows_suppressed=2", "cpu=001 suppressed_rows=2", "missing_thread_identity=1"},
		},
		{
			name: "unassigned CPU fails entire sched switch family",
			rows: []string{
				"INSERT INTO sched_slice VALUES (100, 100, 1, 'S', 40, 1)",
				"INSERT INTO sched_slice VALUES (200, 100, 1, 'R', 41, 2)",
				"INSERT INTO sched_slice VALUES (300, 100, NULL, 'R', 42, 1)",
				"INSERT INTO sched_slice VALUES (400, 100, 'cpu-x', 'R', 43, 2)",
			},
			wantRead: 4, wantEmitted: 0,
			wantSkipped: []string{"family_fail_closed=true", "rows_suppressed=4", "unassigned_cpu_rows=2", "non_integer_cpu=1", "null_cpu=1"},
		},
		{
			name: "typed scalar gates suppress whole CPU lane",
			rows: []string{
				"INSERT INTO sched_slice VALUES (-1, 1, 1, '', 2147483648, 1)",
				"INSERT INTO sched_slice VALUES (0, -1, 1, 'R', 41, 2)",
			},
			wantRead: 2, wantEmitted: 0,
			wantSkipped: []string{"rows_suppressed=2", "invalid_timestamp=1", "invalid_duration=1", "invalid_or_empty_state=1", "invalid_priority=1"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, coverage, _ := exportTraceDBSchedSwitchFixture(t, test.rows)
			if coverage.RowsRead != test.wantRead || coverage.RowsEmitted != test.wantEmitted ||
				strings.Count(body, "sched_switch:") != test.wantEmitted {
				t.Fatalf("sched continuity result mismatch: coverage=%+v\n%s", coverage, body)
			}
			for _, want := range test.wantSkipped {
				if !strings.Contains(coverage.Skipped, want) {
					t.Fatalf("sched continuity coverage missing %q: %+v", want, coverage)
				}
			}
			if test.wantEmitted > 0 && strings.Contains(body, "next_comm=swapper") {
				t.Fatalf("non-idle fixture gained a synthetic idle close:\n%s", body)
			}
		})
	}
}

func TestExportTraceDBSchedSwitchCanonicalIdleChainRoundTrip(t *testing.T) {
	body, coverage, index := exportTraceDBSchedSwitchFixture(t, []string{
		"INSERT INTO sched_slice VALUES (100, 100, 2, 'S', 40, 1)",
		"INSERT INTO sched_slice VALUES (200, 50, 2, 'R', 120, 0)",
		"INSERT INTO sched_slice VALUES (250, 100, 2, 'R', 41, 2)",
	})
	if coverage.RowsRead != 3 || coverage.RowsEmitted != 2 || !strings.Contains(coverage.Skipped, "open_tail_rows=1") {
		t.Fatalf("idle-chain coverage mismatch: %+v", coverage)
	}
	for _, want := range []string{
		"prev_comm=UserA prev_pid=101 prev_prio=40 prev_state=S ==> next_comm=swapper next_pid=0 next_prio=120",
		"prev_comm=swapper prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=UserB next_pid=201 next_prio=41",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("canonical idle chain missing %q:\n%s", want, body)
		}
	}
	var switches []tracequery.Event
	for _, event := range index.Events {
		if event.Type == tracequery.EventSchedSwitch {
			switches = append(switches, event)
		}
	}
	if len(switches) != 2 || switches[0].NextPID != 0 || switches[1].PrevPID != 0 ||
		switches[0].NextComm != "swapper" || switches[1].PrevComm != "swapper" {
		t.Fatalf("canonical idle chain lost in tracequery round trip: %+v", switches)
	}
	if !strings.Contains(coverage.FieldSources["next_identity"], "canonical itid=0 is swapper") {
		t.Fatalf("idle identity provenance missing: %+v", coverage)
	}
}

func TestExportTraceDBSchedSwitchZeroProcessPIDFallsBackToThreadTID(t *testing.T) {
	_, coverage, index := exportTraceDBSchedSwitchFixture(t, []string{
		"INSERT INTO sched_slice VALUES (100, 100, 3, 'S', 40, 1)",
		"INSERT INTO sched_slice VALUES (200, 100, 3, 'R', 41, 3)",
		"INSERT INTO sched_slice VALUES (300, 100, 3, 'R', 42, 2)",
	})
	if coverage.RowsEmitted != 2 || strings.Contains(coverage.Skipped, "invalid_tgid") {
		t.Fatalf("process PID zero should use the thread TID fallback: %+v", coverage)
	}
	found := false
	for _, event := range index.Events {
		if event.Type == tracequery.EventSchedSwitch && event.PrevPID == 301 {
			found = true
			if event.TGID != 301 {
				t.Fatalf("PID-zero process fallback TGID=%d, want thread TID 301: %+v", event.TGID, event)
			}
		}
	}
	if !found {
		t.Fatalf("PID-zero process thread did not publish its real boundary: %+v", index.Events)
	}
}

func TestExportTraceDBSchedSwitchLifecycleCutSuppressesOnlyAffectedCPULane(t *testing.T) {
	tests := []struct {
		name      string
		lifecycle traceDBLifecycleIndex
	}{
		{
			name: "thread cut only",
			lifecycle: traceDBLifecycleIndex{ByTID: map[int64]traceDBLifecycleLane{
				101: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 1, NewIPID: 1}}},
			}},
		},
		{
			name: "process cut only",
			lifecycle: traceDBLifecycleIndex{ByPID: map[int64]traceDBLifecycleLane{
				100: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 1, NewIPID: 1}}},
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, coverage, index := exportTraceDBSchedSwitchFixtureWithLifecycle(t, []string{
				"INSERT INTO sched_slice VALUES (90, 11, 1, 'S', 40, 1)",
				"INSERT INTO sched_slice VALUES (101, 10, 1, 'R', 41, 1)",
				"INSERT INTO sched_slice VALUES (90, 10, 2, 'S', 42, 2)",
				"INSERT INTO sched_slice VALUES (100, 10, 2, 'R', 43, 2)",
			}, test.lifecycle, true)
			if strings.Count(body, "sched_switch:") != 1 || len(index.Events) != 1 || index.Events[0].CPU != 2 {
				t.Fatalf("lifecycle-invalid CPU lane was not isolated from valid sibling:\n%s\n%+v", body, index.Events)
			}
			for _, want := range []string{"rows_suppressed=2", "cpu=001 suppressed_rows=2", "lifecycle_half_open_rejected=1"} {
				if !strings.Contains(coverage.Skipped, want) {
					t.Fatalf("sched lifecycle coverage missing %q: %+v", want, coverage)
				}
			}
			if coverage.RowsRead != 4 || coverage.RowsEmitted != 1 || coverage.FieldSources["lifecycle"] == "" ||
				coverage.FieldSources["source_identity"] == "" {
				t.Fatalf("sched lifecycle accounting/provenance mismatch: %+v", coverage)
			}
			for _, want := range []string{"positive-duration slices, including a final row", "half-open", "zero-duration and NULL-duration open-tail rows"} {
				if !strings.Contains(coverage.FieldSources["lifecycle"], want) {
					t.Fatalf("sched lifecycle provenance missing %q: %+v", want, coverage)
				}
			}
		})
	}
}

func TestExportTraceDBSchedSwitchLifecycleAlignedCutAndFinalInterval(t *testing.T) {
	lifecycle := traceDBLifecycleIndex{
		ByTID: map[int64]traceDBLifecycleLane{
			101: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 1, NewIPID: 1}}},
		},
		ByPID: map[int64]traceDBLifecycleLane{
			100: {Cuts: []traceDBLifecycleBoundary{{TS: 100, NewITID: 1, NewIPID: 1}}},
		},
	}
	body, coverage, _ := exportTraceDBSchedSwitchFixtureWithLifecycle(t, []string{
		"INSERT INTO sched_slice VALUES (90, 10, 1, 'S', 40, 1)",
		"INSERT INTO sched_slice VALUES (100, 10, 1, 'R', 41, 1)",
	}, lifecycle, true)
	if strings.Count(body, "sched_switch:") != 1 || strings.Contains(coverage.Skipped, "lifecycle_") {
		t.Fatalf("cut-aligned old end/new start was rejected:\n%s\n%+v", body, coverage)
	}

	body, coverage, _ = exportTraceDBSchedSwitchFixtureWithLifecycle(t, []string{
		"INSERT INTO sched_slice VALUES (90, 20, 1, 'S', 40, 1)",
		"INSERT INTO sched_slice VALUES (90, 10, 2, 'S', 42, 2)",
		"INSERT INTO sched_slice VALUES (100, NULL, 2, 'R', 43, 2)",
	}, lifecycle, true)
	if strings.Count(body, "sched_switch:") != 1 || !strings.Contains(coverage.Skipped, "cpu=001 suppressed_rows=1") ||
		!strings.Contains(coverage.Skipped, "lifecycle_half_open_rejected=1") {
		t.Fatalf("final positive-duration slice was treated as a point or poisoned sibling:\n%s\n%+v", body, coverage)
	}
}

func TestExportTraceDBSchedSwitchLifecyclePointAndExactIdleRules(t *testing.T) {
	body, coverage, _ := exportTraceDBSchedSwitchFixtureWithLifecycle(t, []string{
		"INSERT INTO sched_slice VALUES (100, 10, 1, 'R', 120, 0)",
		"INSERT INTO sched_slice VALUES (110, NULL, 1, 'R', 120, 0)",
		"INSERT INTO sched_slice VALUES (100, 10, 2, 'S', 40, 1)",
		"INSERT INTO sched_slice VALUES (110, NULL, 2, 'R', 41, 1)",
	}, traceDBLifecycleIndex{}, false)
	if strings.Count(body, "sched_switch:") != 1 || !strings.Contains(body, "prev_comm=swapper") ||
		!strings.Contains(coverage.Skipped, "cpu=002 suppressed_rows=2") ||
		!strings.Contains(coverage.Skipped, "lifecycle_half_open_rejected=1") ||
		!strings.Contains(coverage.Skipped, "lifecycle_point_rejected=1") {
		t.Fatalf("incomplete authority did not preserve exact idle and reject non-idle:\n%s\n%+v", body, coverage)
	}

	_, coverage, _ = exportTraceDBSchedSwitchFixtureWithLifecycle(t, []string{
		"INSERT INTO sched_slice VALUES (100, 0, 1, 'R', 120, 0)",
		"INSERT INTO sched_slice VALUES (100, 10, 1, 'R', 120, 0)",
	}, traceDBLifecycleIndex{GlobalPoison: []int64{100}}, true)
	if coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "lifecycle_point_rejected=1") ||
		!strings.Contains(coverage.Skipped, "lifecycle_half_open_rejected=1") {
		t.Fatalf("zero/positive-duration idle rows bypassed poisoned start point: %+v", coverage)
	}

	body, coverage, _ = exportTraceDBSchedSwitchFixtureWithLifecycle(t, []string{
		"INSERT INTO sched_slice VALUES (90, 20, 1, 'R', 120, 0)",
		"INSERT INTO sched_slice VALUES (110, 10, 2, 'S', 40, 2)",
		"INSERT INTO sched_slice VALUES (120, NULL, 2, 'R', 41, 2)",
	}, traceDBLifecycleIndex{GlobalPoison: []int64{100}}, true)
	if strings.Count(body, "sched_switch:") != 1 || !strings.Contains(coverage.Skipped, "cpu=001 suppressed_rows=1") ||
		!strings.Contains(coverage.Skipped, "lifecycle_half_open_rejected=1") {
		t.Fatalf("positive-duration idle row was downgraded to a start point or poisoned another time range:\n%s\n%+v", body, coverage)
	}

	_, coverage, _ = exportTraceDBSchedSwitchFixtureWithLifecycle(t, []string{
		"INSERT INTO sched_slice VALUES (100, NULL, 1, 'R', 120, NULL)",
		"INSERT INTO sched_slice VALUES (100, NULL, 2, 'R', 120, CAST('0' AS TEXT))",
		"INSERT INTO sched_slice VALUES (100, NULL, 3, 'R', 120, CAST(0 AS REAL))",
		"INSERT INTO sched_slice VALUES (100, NULL, 4, 'R', 120, X'00')",
	}, traceDBLifecycleIndex{}, false)
	if coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "rows_suppressed=4") ||
		!strings.Contains(coverage.Skipped, "invalid_itid=1") {
		t.Fatalf("non-exact/default zero acquired scheduler idle authority: %+v", coverage)
	}
}

func exportTraceDBSchedSwitchFixture(t *testing.T, schedRows []string) (string, TraceDBCoverage, *tracequery.Index) {
	return exportTraceDBSchedSwitchFixtureWithLifecycle(t, schedRows, traceDBLifecycleIndex{}, true)
}

func exportTraceDBSchedSwitchFixtureWithLifecycle(t *testing.T, schedRows []string, lifecycle traceDBLifecycleIndex, complete bool) (string, TraceDBCoverage, *tracequery.Index) {
	t.Helper()
	statements := []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'ProcA')",
		"INSERT INTO process VALUES (2, 200, 'ProcB')",
		"INSERT INTO process VALUES (3, 0, 'ZeroProc')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 101, 1, 'UserA', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 201, 2, 'UserB', 0, 0, 1)",
		"INSERT INTO thread VALUES (3, 301, 3, 'ZeroTGID', 0, 0, 1)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid)",
	}
	statements = append(statements, schedRows...)
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	threadIndex, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	authority := newTraceDBSchedulerAuthority(threadIndex, traceDBLifecycleCollection{
		Lifecycle: lifecycle, CreationComplete: complete, TerminalComplete: complete, ActivityComplete: complete,
	})
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := exportTraceDBSchedSwitch(context.Background(), tdb, sink, authority)
	if err != nil {
		t.Fatalf("export sched switch fixture: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "sched-switch.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := sink.prepareAndWriteForTest(context.Background(), out)
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
	index, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery sched fixture: %v", err)
	}
	return string(bodyBytes), coverage, index
}

func TestExportTraceDBThreadRegistrationPreservesZeroStart(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (100000)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (2, 101, 1, 'zero-start', 0, 0, 1)",
	})
	outPath := filepath.Join(t.TempDir(), "zero-start.systrace")
	_, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export zero-start registration: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	if !strings.Contains(body, "0.000000: task_rename: pid=101") || strings.Contains(body, "0.000100: task_rename: pid=101") {
		t.Fatalf("valid thread.start_ts=0 was rewritten to trace_range.start_ts:\n%s", body)
	}
}

func TestExportTraceDBThreadRegistrationFallsBackForUnknownOrTaintedHint(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (100000)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (2, 101, 1, 'null-start', NULL, 0, 1)",
		"INSERT INTO thread VALUES (3, 102, 1, 'text-start', CAST(0 AS TEXT), 0, 1)",
	})
	outPath := filepath.Join(t.TempDir(), "metadata-start.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export metadata-only thread starts: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, tid := range []string{"pid=101", "pid=102"} {
		if !strings.Contains(body, "0.000100: task_rename: "+tid) {
			t.Fatalf("unknown/tainted registration hint did not fall back to capture start for %s:\n%s", tid, body)
		}
	}
	for _, item := range result.Coverage {
		if item.Family == "resolver" && item.Table == "thread" && !strings.Contains(item.Skipped, "metadata ignored for hard identity") {
			t.Fatalf("tainted registration hint was not disclosed: %+v", item)
		}
	}
}

func TestExportTraceDBThreadRegistrationDoesNotSynthesizeUnknownCaptureStart(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'demo')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (2, 101, 1, 'unknown-start', NULL, 0, 1)",
	})
	outPath := filepath.Join(t.TempDir(), "unknown-capture-start.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("export unknown capture start: %v", err)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if body := string(bodyBytes); strings.Contains(body, "task_rename: pid=101") || strings.Contains(body, "B|100|demo") {
		t.Fatalf("unknown capture start was synthesized as t=0 registration:\n%s", body)
	}
	found := false
	for _, item := range result.Coverage {
		if item.Family == "metadata" && item.Table == "thread" {
			found = true
			if item.RowsEmitted != 0 || !strings.Contains(item.Skipped, "1 thread registration(s) skipped") {
				t.Fatalf("unknown registration timestamp was not disclosed: %+v", item)
			}
		}
	}
	if !found {
		t.Fatalf("missing thread registration coverage: %+v", result.Coverage)
	}
}

func TestExportTraceDBThreadRegistrationSanitizesDisplayOnlyNames(t *testing.T) {
	oversized := strings.Repeat("x", maxTraceDBIdentityDisplayBytes+1)
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'bad process')",
		"INSERT INTO process VALUES (2, 200, 'control-process')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (2, 101, 1, '" + oversized + "', 0, 0, 1)",
		"INSERT INTO thread VALUES (3, 201, 2, 'control-thread', 0, 0, 1)",
	})
	outPath := filepath.Join(t.TempDir(), "display-only-name.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, outPath)
	if err != nil {
		t.Fatalf("display-only name must not fail the conversion: %v", err)
	}
	assertCoverageEmitted(t, result.Coverage, "metadata", "thread", 6)
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	for _, want := range []string{"task_rename: pid=101", "task_rename: pid=201", "control-process"} {
		if !strings.Contains(body, want) {
			t.Fatalf("display sanitizer lost identity or valid sibling %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "bad process") || strings.Contains(body, strings.Repeat("x", 64)) {
		t.Fatalf("unsafe display name leaked into a physical output line:\n%s", body)
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
