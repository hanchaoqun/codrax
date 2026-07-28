package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceDBRawSchedSwitchLiteJoinEnrichesUniqueBoundaryWithoutDuplicate(t *testing.T) {
	const nextInfo = uint64(0x001100003fff)
	raw := traceDBRawSchedSwitchLiteRecord{
		TimestampNS: 1200, CPU: 7, HeaderPID: 101, Flags: 1, PreemptCount: 3,
		PrevTID: 101, PrevPriority: 99, PrevState: 1,
		NextTID: 201, NextPriority: 52, NextInfo: nextInfo,
	}
	body, schedulerCoverage, joinCoverage, index := exportTraceDBSchedSwitchLiteJoinFixture(t, []traceDBRawSchedSwitchLiteRecord{raw})
	if strings.Count(body, "sched_switch:") != 1 ||
		!strings.Contains(body, "prev_pid=101 prev_prio=99 prev_state=S") ||
		!strings.Contains(body, "next_pid=201 next_prio=52 next_info=") ||
		!strings.Contains(body, "codrax_next_info_raw=0x0000001100003fff") ||
		!strings.Contains(body, "codrax_next_info_source=official_raw_sched_switch_lite") {
		t.Fatalf("unique raw lite boundary was not enriched exactly once:\n%s", body)
	}
	if schedulerCoverage.RowsEmitted != 1 || joinCoverage.RowsEmitted != 1 ||
		joinCoverage.Metrics["db_boundaries_enriched"] != 1 ||
		joinCoverage.Metadata["join_state"] != "published_unique_exact_enrichment" ||
		joinCoverage.Metadata["physical_event_contract"] != "enrich_existing_db_boundary_only; duplicate_events=0" {
		t.Fatalf("join coverage drifted: scheduler=%+v join=%+v", schedulerCoverage, joinCoverage)
	}
	if index == nil || len(index.Events) != 1 || index.Events[0].NextInfo == "" {
		t.Fatalf("tracequery did not consume the enriched next_info: %+v", index)
	}
}

func TestTraceDBRawSchedSwitchLiteJoinTreatsCommonPIDAsNonIdentityEnvelope(t *testing.T) {
	raw := traceDBRawSchedSwitchLiteRecord{
		TimestampNS: 1200, CPU: 7, HeaderPID: 32788, Flags: 1, PreemptCount: 3,
		PrevTID: 101, PrevPriority: 99, PrevState: 1,
		NextTID: 201, NextPriority: 52, NextInfo: 0x3fff,
	}
	body, schedulerCoverage, joinCoverage, _ := exportTraceDBSchedSwitchLiteJoinFixture(
		t, []traceDBRawSchedSwitchLiteRecord{raw})
	if strings.Count(body, "sched_switch:") != 1 ||
		!strings.Contains(body, "prev_pid=101 prev_prio=99 prev_state=S") ||
		!strings.Contains(body, "codrax_next_info_source=official_raw_sched_switch_lite") ||
		strings.Contains(body, "32788") {
		t.Fatalf("common_pid changed or blocked the exact DB scheduler boundary:\n%s", body)
	}
	if schedulerCoverage.RowsEmitted != 1 || joinCoverage.RowsEmitted != 1 ||
		joinCoverage.Metrics["raw_records_common_pid_differs_from_prev_tid"] != 1 ||
		joinCoverage.Metrics["raw_records_key_rejected"] != 0 {
		t.Fatalf("common_pid envelope accounting drifted: scheduler=%+v join=%+v",
			schedulerCoverage, joinCoverage)
	}
}

func TestTraceDBRawSchedSwitchLiteJoinFailsClosedOnBodyIdentityStateAndMultiplicity(t *testing.T) {
	base := traceDBRawSchedSwitchLiteRecord{
		TimestampNS: 1200, CPU: 7, HeaderPID: 101,
		PrevTID: 101, PrevPriority: 99, PrevState: 1,
		NextTID: 201, NextPriority: 52, NextInfo: 0x3fff,
	}
	tests := []struct {
		name string
		rows []traceDBRawSchedSwitchLiteRecord
	}{
		{
			name: "state_mismatch",
			rows: func() []traceDBRawSchedSwitchLiteRecord {
				row := base
				row.PrevState = 2
				return []traceDBRawSchedSwitchLiteRecord{row}
			}(),
		},
		{
			name: "unsupported_state",
			rows: func() []traceDBRawSchedSwitchLiteRecord {
				row := base
				row.PrevState = 7
				return []traceDBRawSchedSwitchLiteRecord{row}
			}(),
		},
		{
			name: "duplicate_raw_key",
			rows: []traceDBRawSchedSwitchLiteRecord{base, base},
		},
		{
			name: "next_priority_mismatch",
			rows: func() []traceDBRawSchedSwitchLiteRecord {
				row := base
				row.NextPriority = 53
				return []traceDBRawSchedSwitchLiteRecord{row}
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, schedulerCoverage, joinCoverage, _ := exportTraceDBSchedSwitchLiteJoinFixture(t, test.rows)
			if strings.Count(body, "sched_switch:") != 1 ||
				!strings.Contains(body, "prev_pid=101 prev_prio=42 prev_state=S") ||
				strings.Contains(body, "codrax_next_info_") {
				t.Fatalf("unproven raw row changed the DB boundary:\n%s", body)
			}
			if schedulerCoverage.RowsEmitted != 1 || joinCoverage.RowsEmitted != 0 ||
				joinCoverage.Metadata["join_state"] != "complete_no_unique_match" {
				t.Fatalf("unproven join coverage drifted: scheduler=%+v join=%+v",
					schedulerCoverage, joinCoverage)
			}
			if test.name == "unsupported_state" &&
				joinCoverage.Metrics["raw_records_key_rejected_unsupported_prev_state"] != 1 {
				t.Fatalf("typed key rejection reason missing: %+v", joinCoverage)
			}
		})
	}
}

func TestTraceDBRawSchedSwitchLiteJoinPreservesUnknownFutureTailAsReceiptOnly(t *testing.T) {
	raw := traceDBRawSchedSwitchLiteRecord{
		TimestampNS: 1200, CPU: 7, HeaderPID: 101,
		PrevTID: 101, PrevPriority: 99, PrevState: 1,
		NextTID: 201, NextPriority: 52, NextInfo: uint64(1)<<63 | 0x3fff,
	}
	body, _, joinCoverage, _ := exportTraceDBSchedSwitchLiteJoinFixture(t, []traceDBRawSchedSwitchLiteRecord{raw})
	if !strings.Contains(body, "next_info=3fff,0,0,0,0,0") ||
		!strings.Contains(body, "codrax_next_info_raw=0x8000000000003fff") ||
		joinCoverage.RowsEmitted != 1 {
		t.Fatalf("future packed tail was guessed or lost:\n%s\n%+v", body, joinCoverage)
	}
}

func TestTraceDBRawSchedSwitchLiteJoinDoesNotClaimCompletionWithoutDBCensus(t *testing.T) {
	raw := traceDBRawSchedSwitchLiteRecord{
		TimestampNS: 1200, CPU: 7, HeaderPID: 101,
		PrevTID: 101, PrevPriority: 99, PrevState: 1,
		NextTID: 201, NextPriority: 52, NextInfo: 0x3fff,
	}
	join := newTraceDBRawSchedSwitchLiteJoin(&traceDBSourceNameInventory{
		RawDecode: TraceDBCoverage{
			Role: "diagnostic_ledger",
			Metadata: map[string]string{
				"decode_state": "strict_target_ledger_complete",
				"scheduler_lite_format_geometry_witnesses": "sched_switch_lite#1[prev_pid@8:4]",
			},
			Metrics: map[string]int64{"target_sched_switch_lite_body_admitted": 1},
		},
		RawSwitchLite: []traceDBRawSchedSwitchLiteRecord{raw},
	})
	coverage, err := join.finalize()
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metadata["join_state"] != "withheld_db_scheduler_census_unavailable" ||
		coverage.Metadata["scheduler_lite_format_geometry_witnesses"] !=
			"sched_switch_lite#1[prev_pid@8:4]" ||
		coverage.Metadata["source_decoder_census"] != "body_admitted=1" ||
		coverage.RowsEmitted != 0 {
		t.Fatalf("missing DB census was reported as a completed join: %+v", coverage)
	}
}

func TestTraceDBRawSchedSwitchLiteEndStatePinsUpstreamTable(t *testing.T) {
	tests := map[uint64]string{
		0: "R", 1: "S", 2: "D", 21: "D-IO", 22: "D-NIO", 3: "Running",
		4: "T", 8: "t", 16: "X", 32: "Z", 64: "P", 128: "I",
		130: "DK", 131: "DK-IO", 132: "DK-NIO", 136: "TK",
		256: "R+", 2048: "R+", 2049: "R-B", 4096: "S", 0x8000: "U",
	}
	for raw, want := range tests {
		if got, ok := traceDBRawSchedSwitchLiteEndState(raw); !ok || got != want {
			t.Fatalf("end state raw=%d got=%q ok=%t want=%q", raw, got, ok, want)
		}
	}
	if got, ok := traceDBRawSchedSwitchLiteEndState(7); ok || got != "" {
		t.Fatalf("unknown end state acquired a mapping: got=%q ok=%t", got, ok)
	}
}

func TestTraceDBRawSchedSwitchLiteJoinCoverageIsAttachedToExport(t *testing.T) {
	body, _, joinCoverage, _ := exportTraceDBSchedSwitchLiteJoinFixture(t, []traceDBRawSchedSwitchLiteRecord{{
		TimestampNS: 1200, CPU: 7, HeaderPID: 101,
		PrevTID: 101, PrevPriority: 99, PrevState: 1,
		NextTID: 201, NextPriority: 52, NextInfo: 0x3fff,
	}})
	if body == "" || joinCoverage.Family != "source_rawtrace_scheduler_lite_join" ||
		joinCoverage.Table != "__raw_vs_db_sched_switch__" ||
		joinCoverage.Role != "query_ready_enrichment" {
		t.Fatalf("production scheduler exporter did not attach typed join coverage: %+v", joinCoverage)
	}
	source, err := os.ReadFile("streamerdb_export.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source),
		"coverage = append(coverage, cloneTraceDBCoverage(tdb.rawSchedSwitchJoinCoverage))") {
		t.Fatal("final trace DB export no longer attaches scheduler-lite join coverage")
	}
}

func exportTraceDBSchedSwitchLiteJoinFixture(
	t *testing.T,
	raw []traceDBRawSchedSwitchLiteRecord,
) (string, TraceDBCoverage, TraceDBCoverage, *tracequery.Index) {
	t.Helper()
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'ProcA')",
		"INSERT INTO process VALUES (2, 200, 'ProcB')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 101, 1, 'UserA', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 201, 2, 'UserB', 0, 0, 1)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
		"INSERT INTO sched_slice VALUES (1000, 200, 7, 'S', 42, 1)",
		"INSERT INTO sched_slice VALUES (1200, NULL, 7, 'R', 52, 2)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	tdb.sourceNameInventory = &traceDBSourceNameInventory{
		RawDecode: TraceDBCoverage{
			Role: "diagnostic_ledger",
			Metadata: map[string]string{
				"decode_state": "strict_target_ledger_complete",
			},
			Metrics: map[string]int64{
				"target_sched_switch_lite_body_admitted": int64(len(raw)),
			},
		},
		RawSwitchLite: append([]traceDBRawSchedSwitchLiteRecord(nil), raw...),
	}
	threadIndex, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	authority := newTraceDBSchedulerAuthority(threadIndex, traceDBLifecycleCollection{
		Lifecycle:        traceDBLifecycleIndex{},
		CreationComplete: true,
		TerminalComplete: true,
		ActivityComplete: true,
	})
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	schedulerCoverage, err := exportTraceDBSchedSwitch(context.Background(), tdb, sink, authority)
	if err != nil {
		t.Fatalf("export scheduler-lite join fixture: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "sched-switch-lite-join.systrace")
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
		t.Fatalf("tracequery scheduler-lite join fixture: %v", err)
	}
	return string(bodyBytes), schedulerCoverage, tdb.rawSchedSwitchJoinCoverage, index
}
