package hitraceconv

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func traceDBRawBlockedRecoveryTestLedger(admitted int64) TraceDBCoverage {
	coverage := newTraceDBRawBlockedKeyCoverage()
	coverage.Found = true
	coverage.Metrics = map[string]int64{}
	coverage.Metadata["ledger_state"] = "exact_content_multiset_subset"
	coverage.Metrics["raw_rows_absent_db_identity_admitted"] = admitted
	return coverage
}

func traceDBRawBlockedRecoveryTestRow() traceDBRawBlockedRecoveryRow {
	return traceDBRawBlockedRecoveryRow{
		Raw: traceDBRawBlockedRecord{
			TimestampNS: 1_234_567_890, CPU: 2, HeaderPID: 201,
			Flags: 1, PreemptCount: 2, TargetTID: 101, IOWait: 1,
			CallerRaw: 0x1234, CNodeIndex: 7, CNodeKnown: true,
			Delay: 11, DelayKnown: true,
		},
		TargetThread:  traceDBThread{ITID: 1, TID: 101, IPID: 1, Name: "target-thread"},
		TargetProcess: traceDBProcess{IPID: 1, PID: 100, Name: "target-process"},
		HeaderThread:  traceDBThread{ITID: 2, TID: 201, IPID: 2, Name: "header-thread"},
		HeaderProcess: traceDBProcess{IPID: 2, PID: 200, Name: "header-process"},
	}
}

func TestPublishTraceDBRawBlockedRecoveryPreservesExactEnvelopeAndBody(t *testing.T) {
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sink.cleanup(); err != nil {
			t.Errorf("cleanup raw blocked recovery sink: %v", err)
		}
	}()

	coverage, err := publishTraceDBRawBlockedRecovery(
		context.Background(), sink, traceDBRawBlockedRecoveryTestLedger(1),
		[]traceDBRawBlockedRecoveryRow{traceDBRawBlockedRecoveryTestRow()},
	)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Role != "query_ready_export" || coverage.RowsRead != 1 ||
		coverage.RowsEmitted != 1 ||
		coverage.Metadata["publication_state"] != "published_exact_raw_only_content_cohorts" ||
		sink.stats.RowsAccepted != 1 || len(sink.rows) != 1 {
		t.Fatalf("raw blocked recovery publication mismatch: coverage=%+v stats=%+v rows=%+v",
			coverage, sink.stats, sink.rows)
	}
	line := sink.rows[0].line
	for _, want := range []string{
		"header-thread-201", "(  200) [002] d..2",
		"1.234568: sched_blocked_reason: pid=101 iowait=1",
		"caller=0x1234 cnode_idx=7 delay=11",
		"source=official_rawtrace_rpd2a raw_db_content_cohort=absent",
	} {
		if !strings.Contains(line, want) {
			t.Fatalf("raw blocked recovery line missing %q:\n%s", want, line)
		}
	}
	if sink.rows[0].tsNS != 1_234_568_000 {
		t.Fatalf("sort timestamp did not match rounded wire timestamp: %+v", sink.rows[0])
	}
}

func TestPublishTraceDBRawBlockedRecoveryDoesNotInventKernelTGID(t *testing.T) {
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sink.cleanup(); err != nil {
			t.Errorf("cleanup kernel raw blocked recovery sink: %v", err)
		}
	}()
	row := traceDBRawBlockedRecoveryTestRow()
	row.HeaderProcess.PID = 0
	coverage, err := publishTraceDBRawBlockedRecovery(
		context.Background(), sink, traceDBRawBlockedRecoveryTestLedger(1),
		[]traceDBRawBlockedRecoveryRow{row},
	)
	if err != nil || coverage.RowsEmitted != 1 || len(sink.rows) != 1 {
		t.Fatalf("kernel recovery publication mismatch: coverage=%+v err=%v rows=%+v",
			coverage, err, sink.rows)
	}
	if !strings.Contains(sink.rows[0].line, "(-----)") ||
		strings.Contains(sink.rows[0].line, "(  201)") {
		t.Fatalf("kernel header gained fabricated TGID:\n%s", sink.rows[0].line)
	}
}

func TestPublishTraceDBRawBlockedRecoveryFailsClosed(t *testing.T) {
	t.Run("key ledger unavailable", func(t *testing.T) {
		coverage, err := publishTraceDBRawBlockedRecovery(
			context.Background(), nil, newTraceDBRawBlockedKeyCoverage(), nil,
		)
		if err != nil || coverage.RowsEmitted != 0 ||
			coverage.Metadata["publication_state"] != "withheld_key_ledger_not_exact" {
			t.Fatalf("inexact key ledger gained publication authority: coverage=%+v err=%v", coverage, err)
		}
	})

	t.Run("identity census mismatch", func(t *testing.T) {
		coverage, err := publishTraceDBRawBlockedRecovery(
			context.Background(), nil, traceDBRawBlockedRecoveryTestLedger(1), nil,
		)
		if err != nil || coverage.RowsEmitted != 0 ||
			coverage.Metadata["publication_state"] != "withheld_identity_census_mismatch" {
			t.Fatalf("identity census mismatch gained publication authority: coverage=%+v err=%v", coverage, err)
		}
	})

	t.Run("no eligible raw-only cohort", func(t *testing.T) {
		coverage, err := publishTraceDBRawBlockedRecovery(
			context.Background(), nil, traceDBRawBlockedRecoveryTestLedger(0), nil,
		)
		if err != nil || coverage.RowsRead != 0 || coverage.RowsEmitted != 0 ||
			coverage.Metadata["publication_state"] != "complete_no_eligible_raw_only_row" {
			t.Fatalf("zero eligible cohort was not typed: coverage=%+v err=%v", coverage, err)
		}
	})

	t.Run("cancelled before publication", func(t *testing.T) {
		sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := sink.cleanup(); err != nil {
				t.Errorf("cleanup cancelled raw blocked recovery sink: %v", err)
			}
		}()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		coverage, err := publishTraceDBRawBlockedRecovery(
			ctx, sink, traceDBRawBlockedRecoveryTestLedger(1),
			[]traceDBRawBlockedRecoveryRow{traceDBRawBlockedRecoveryTestRow()},
		)
		if !errors.Is(err, context.Canceled) || coverage.RowsEmitted != 0 ||
			sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 {
			t.Fatalf("cancelled recovery partially published: coverage=%+v err=%v stats=%+v",
				coverage, err, sink.stats)
		}
	})

	t.Run("identity tuple mutated after ledger", func(t *testing.T) {
		sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := sink.cleanup(); err != nil {
				t.Errorf("cleanup invalid identity raw blocked recovery sink: %v", err)
			}
		}()
		row := traceDBRawBlockedRecoveryTestRow()
		row.HeaderThread.TID = 202
		coverage, err := publishTraceDBRawBlockedRecovery(
			context.Background(), sink, traceDBRawBlockedRecoveryTestLedger(1),
			[]traceDBRawBlockedRecoveryRow{row},
		)
		if err == nil || coverage.RowsEmitted != 0 ||
			sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 {
			t.Fatalf("mutated identity tuple partially published: coverage=%+v err=%v stats=%+v",
				coverage, err, sink.stats)
		}
	})
}

func TestTraceDBRawBlockedKeyLedgerReturnsOnlyDBDisjointIdentityAdmittedRows(t *testing.T) {
	rawRows := []traceDBRawBlockedRecord{
		{TimestampNS: 100, CPU: 1, HeaderPID: 201, TargetTID: 101, IOWait: 1, CallerRaw: 0x123},
		{TimestampNS: 101, CPU: 2, HeaderPID: 201, TargetTID: 101, IOWait: 0, CallerRaw: 0x456},
		{TimestampNS: 102, CPU: 3, HeaderPID: 201, TargetTID: 999, IOWait: 0, CallerRaw: 0x789},
	}
	inventory := &traceDBSourceNameInventory{
		RawDecode: TraceDBCoverage{
			Found: true,
			Metrics: map[string]int64{
				"target_sched_blocked_reason_body_admitted": 3,
			},
			Metadata: map[string]string{"decode_state": "strict_target_ledger_complete"},
		},
		RawBlocked: rawRows,
	}
	dbRows := []traceDBPreparedBlockedReason{{
		Row:    traceDBBlockedStateRow{TID: 101},
		IOWait: 1, Caller: "unknown", CallerRaw: "0x123", CallerQuality: "opaque",
	}}
	coverage, rows := traceDBRawBlockedKeyLedger(
		inventory, dbRows, traceDBRawBlockedKeyTestAuthority(),
	)
	if len(rows) != 1 || rows[0].Raw != rawRows[1] ||
		rows[0].TargetThread.TID != 101 || rows[0].HeaderThread.TID != 201 ||
		coverage.Metrics["raw_rows_withheld_overlap_cohort"] != 1 ||
		coverage.Metrics["raw_rows_absent_db_identity_rejected_target_absent_or_lifecycle_absent"] != 1 {
		t.Fatalf("recovery selection leaked overlap or unresolved namespace row: coverage=%+v rows=%+v",
			coverage, rows)
	}
}
