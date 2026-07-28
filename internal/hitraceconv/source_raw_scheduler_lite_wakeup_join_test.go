package hitraceconv

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceDBRawSchedWakeupLiteJoinUsesExactPhysicalCPUAndPriorityWithoutDuplicate(t *testing.T) {
	raw := traceDBRawSchedWakeupLiteRecord{
		TimestampNS: 1000, CPU: 5, HeaderPID: 200, Flags: 1, PreemptCount: 3,
		TargetTID: 100, Priority: 159, TargetCPU: 7,
	}
	body, schedulerCoverage, joinCoverage, index := exportTraceDBSchedWakeupLiteJoinFixture(
		t, wakeupLiteJoinFixtureOptions{Raw: []traceDBRawSchedWakeupLiteRecord{raw}, DBRawITID: 1})
	if strings.Count(body, "sched_wakeup:") != 1 ||
		!strings.Contains(body, "[005]") ||
		!strings.Contains(body, "comm=app pid=100 prio=159 target_cpu=007") ||
		!strings.Contains(body, "codrax_wakeup_source=official_raw_sched_wakeup_lite") ||
		strings.Contains(body, "codrax_prio_source=") {
		t.Fatalf("unique raw wakeup lite edge was not enriched exactly once:\n%s", body)
	}
	wakeup := requireWakeupEvent(t, index)
	if wakeup.CPU != 5 || wakeup.PID != 200 || wakeup.WakeePID != 100 ||
		wakeup.TargetCPU != 7 || wakeup.WakeePrio != 159 ||
		wakeup.WakeePrioritySource() != "" {
		t.Fatalf("exact raw wakeup authority was lost after tracequery round trip: %+v", wakeup)
	}
	mainCoverage := requireWakeupCoverage(t, schedulerCoverage)
	if mainCoverage.RowsEmitted != 1 ||
		mainCoverage.Metrics["wakeup_edges_preserved_cpu_unavailable"] != 0 {
		t.Fatalf("exact raw wakeup was counted as an inferred/CPU-free edge: %+v", mainCoverage)
	}
	if joinCoverage.RowsEmitted != 1 ||
		joinCoverage.Metrics["db_edges_enriched"] != 1 ||
		joinCoverage.Metadata["join_state"] != "published_unique_exact_enrichment" ||
		joinCoverage.Metadata["physical_event_contract"] != "enrich_existing_db_wakeup_only; duplicate_events=0" {
		t.Fatalf("wakeup-lite join coverage drifted: %+v", joinCoverage)
	}
}

func TestTraceDBRawSchedWakeupLiteJoinFailsClosedAndKeepsExistingFallback(t *testing.T) {
	base := traceDBRawSchedWakeupLiteRecord{
		TimestampNS: 1000, CPU: 5, HeaderPID: 200,
		TargetTID: 100, Priority: 159, TargetCPU: 7,
	}
	tests := []struct {
		name string
		opts wakeupLiteJoinFixtureOptions
	}{
		{
			name: "namespace_or_header_pid_not_proven",
			opts: wakeupLiteJoinFixtureOptions{Raw: func() []traceDBRawSchedWakeupLiteRecord {
				row := base
				row.HeaderPID = 32788
				return []traceDBRawSchedWakeupLiteRecord{row}
			}(), DBRawITID: 1},
		},
		{
			name: "wakee_pid_mismatch",
			opts: wakeupLiteJoinFixtureOptions{Raw: func() []traceDBRawSchedWakeupLiteRecord {
				row := base
				row.TargetTID = 101
				return []traceDBRawSchedWakeupLiteRecord{row}
			}(), DBRawITID: 1},
		},
		{
			name: "target_cpu_mismatch",
			opts: wakeupLiteJoinFixtureOptions{Raw: func() []traceDBRawSchedWakeupLiteRecord {
				row := base
				row.TargetCPU = 8
				return []traceDBRawSchedWakeupLiteRecord{row}
			}(), DBRawITID: 1},
		},
		{
			name: "duplicate_raw_key",
			opts: wakeupLiteJoinFixtureOptions{
				Raw: []traceDBRawSchedWakeupLiteRecord{base, base}, DBRawITID: 1,
			},
		},
		{
			name: "exact_sched_wakeup_source_also_present",
			opts: wakeupLiteJoinFixtureOptions{
				Raw: []traceDBRawSchedWakeupLiteRecord{base}, ExactWakeupRecords: 1, DBRawITID: 1,
			},
		},
		{
			name: "bytrace_waker_shaped_db_raw_row",
			opts: wakeupLiteJoinFixtureOptions{
				Raw: []traceDBRawSchedWakeupLiteRecord{base}, DBRawITID: 2,
			},
		},
		{
			name: "nonpositive_priority_not_query_ready",
			opts: wakeupLiteJoinFixtureOptions{Raw: func() []traceDBRawSchedWakeupLiteRecord {
				row := base
				row.Priority = 0
				return []traceDBRawSchedWakeupLiteRecord{row}
			}(), DBRawITID: 1},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, schedulerCoverage, joinCoverage, index :=
				exportTraceDBSchedWakeupLiteJoinFixture(t, test.opts)
			if strings.Count(body, "sched_wakeup:") != 1 ||
				!strings.Contains(body, "[002]") ||
				!strings.Contains(body, "comm=app pid=100 prio=42 target_cpu=007 codrax_prio_source=inferred_next_sched_slice") ||
				strings.Contains(body, "codrax_wakeup_source=official_raw_sched_wakeup_lite") {
				t.Fatalf("unproven raw lite record changed the existing fallback:\n%s", body)
			}
			wakeup := requireWakeupEvent(t, index)
			if wakeup.CPU != 2 || wakeup.WakeePrio != 42 ||
				wakeup.WakeePrioritySource() != tracequery.WakeePrioritySourceInferredNextSchedSlice {
				t.Fatalf("existing fallback authority drifted: %+v", wakeup)
			}
			if requireWakeupCoverage(t, schedulerCoverage).RowsEmitted != 1 ||
				joinCoverage.RowsEmitted != 0 ||
				joinCoverage.Metadata["join_state"] == "published_unique_exact_enrichment" {
				t.Fatalf("unproven wakeup join coverage drifted: %+v", joinCoverage)
			}
		})
	}
}

func TestTraceDBRawSchedWakeupLiteJoinDoesNotClaimCompletionWithoutDBCensus(t *testing.T) {
	raw := traceDBRawSchedWakeupLiteRecord{
		TimestampNS: 1000, CPU: 5, HeaderPID: 200,
		TargetTID: 100, Priority: 159, TargetCPU: 7,
	}
	join := newTraceDBRawSchedWakeupLiteJoin(&traceDBSourceNameInventory{
		RawDecode: TraceDBCoverage{
			Role:     "diagnostic_ledger",
			Metadata: map[string]string{"decode_state": "strict_target_ledger_complete"},
			Metrics:  map[string]int64{"target_sched_wakeup_lite_body_admitted": 1},
		},
		RawWakeupLite: []traceDBRawSchedWakeupLiteRecord{raw},
	})
	coverage, err := join.finalize()
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metadata["join_state"] != "withheld_db_scheduler_census_unavailable" ||
		coverage.RowsEmitted != 0 {
		t.Fatalf("missing DB census was reported as a completed wakeup join: %+v", coverage)
	}
}

func TestTraceDBRawSchedWakeupLiteJoinCoverageIsAttachedToExport(t *testing.T) {
	source, err := os.ReadFile("streamerdb_export.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source),
		"coverage = append(coverage, cloneTraceDBCoverage(tdb.rawSchedWakeupJoinCoverage))") {
		t.Fatal("final trace DB export no longer attaches scheduler-lite wakeup join coverage")
	}
}

type wakeupLiteJoinFixtureOptions struct {
	Raw                []traceDBRawSchedWakeupLiteRecord
	ExactWakeupRecords int64
	DBRawITID          int64
}

func exportTraceDBSchedWakeupLiteJoinFixture(
	t *testing.T,
	options wakeupLiteJoinFixtureOptions,
) (string, []TraceDBCoverage, TraceDBCoverage, *tracequery.Index) {
	t.Helper()
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'App')",
		"INSERT INTO process VALUES (2, 200, 'Worker')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 100, 1, 'app', 0, 1, 1)",
		"INSERT INTO thread VALUES (2, 200, 2, 'waker', 0, 1, 1)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
		"INSERT INTO sched_slice VALUES (1200, NULL, 7, 'R', 42, 1)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT, value REAL)",
		"INSERT INTO instant VALUES (1000, 'sched_wakeup', 1, 2, 'itid', NULL)",
		"CREATE TABLE raw (id INT, ts INT, name TEXT, cpu INT, itid INT)",
		fmt.Sprintf("INSERT INTO raw VALUES (1, 1000, 'sched_wakeup', 7, %d)", options.DBRawITID),
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (2, 900, 200, 2, 'Running')",
		"CREATE TABLE callstack (ts INT, itid INT, callid INT)",
		"CREATE TABLE syscall (ts INT, itid INT)",
		"CREATE TABLE native_hook (start_ts INT, itid INT)",
		"CREATE TABLE frame_slice (ts INT, itid INT)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	tdb.sourceNameInventory = &traceDBSourceNameInventory{
		RawDecode: TraceDBCoverage{
			Role:     "diagnostic_ledger",
			Metadata: map[string]string{"decode_state": "strict_target_ledger_complete"},
			Metrics: map[string]int64{
				"target_sched_wakeup_lite_body_admitted": int64(len(options.Raw)),
				"target_sched_wakeup_records":            options.ExactWakeupRecords,
			},
		},
		RawWakeupLite: append([]traceDBRawSchedWakeupLiteRecord(nil), options.Raw...),
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 4)
	if err != nil {
		t.Fatal(err)
	}
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, _, err := exportTraceDBSchedulerFamilies(context.Background(), tdb, sink, syncSpans)
	if err != nil {
		t.Fatalf("export scheduler-lite wakeup fixture: %v", err)
	}
	coverage, _, _ = finalizeTraceDBTestSyncSpans(t, sink, syncSpans, coverage)
	outPath := filepath.Join(t.TempDir(), "scheduler-lite-wakeup-join.systrace")
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
		t.Fatalf("tracequery scheduler-lite wakeup fixture: %v", err)
	}
	return string(bodyBytes), coverage, tdb.rawSchedWakeupJoinCoverage, index
}

func requireWakeupEvent(t *testing.T, index *tracequery.Index) tracequery.Event {
	t.Helper()
	if index != nil {
		for _, event := range index.Events {
			if event.Type == tracequery.EventSchedWakeup {
				return event
			}
		}
	}
	t.Fatalf("sched_wakeup event missing: %+v", index)
	return tracequery.Event{}
}
