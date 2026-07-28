package hitraceconv

import (
	"context"
	"strings"
	"testing"
)

func traceDBRawAsyncFixture(t *testing.T) (
	*traceDB,
	traceDBSchedulerAuthority,
	traceDBSchedulerRunningIndex,
) {
	t.Helper()
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'begin-process')",
		"INSERT INTO process VALUES (2, 300, 'finish-process')",
		"CREATE TABLE thread (itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 101, 1, 'begin-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 301, 2, 'finish-thread', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (1, 0, 3000, 2, 'Running')",
		"CREATE TABLE callstack (id, ts, dur, itid, callid, name, flag, cookie, chainId, depth)",
		"INSERT INTO callstack VALUES (1, 1000, 1000, 1, NULL, 'official-async', NULL, 9, NULL, 0)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	identities, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		tdb.close()
		t.Fatal(err)
	}
	intervals, integrity, _, err := tdb.loadRunningIntervals(context.Background(), identities)
	if err != nil {
		tdb.close()
		t.Fatal(err)
	}
	authority := traceDBSchedulerAuthority{
		identities:  identities,
		lifecycle:   traceDBLifecycleIndex{},
		initialized: true,
		complete:    true,
	}
	return tdb, authority,
		newTraceDBSchedulerRunningIndex(authority, intervals, integrity, nil)
}

func traceDBRawAsyncRecords(end uint64) []traceDBRawMarkerRecord {
	return []traceDBRawMarkerRecord{
		{
			PhysicalOrdinal: 1, TimestampNS: 1000, CPU: 2, HeaderPID: 101,
			Flags: 1, PreemptCount: 2, Buffer: "S|100|official-async|9",
			Action: "S", PayloadPID: 100, Name: "official-async", Value: "9",
			Admitted: true,
		},
		{
			PhysicalOrdinal: 2, TimestampNS: end, CPU: 3, HeaderPID: 301,
			Flags: 4, PreemptCount: 1, Buffer: "F|100|official-async|9",
			Action: "F", PayloadPID: 100, Name: "official-async", Value: "9",
			Admitted: true,
		},
	}
}

func traceDBRawAsyncInventory(rows []traceDBRawMarkerRecord) *traceDBSourceNameInventory {
	return &traceDBSourceNameInventory{
		RawDecode: TraceDBCoverage{
			Metadata: map[string]string{
				"decode_state": "strict_target_ledger_complete",
			},
			Metrics: map[string]int64{
				"target_marker_async_records_retained": int64(len(rows)),
			},
		},
		RawMarkers: rows,
	}
}

func TestTraceDBRawAsyncLedgerPairsIndependentPhysicalEmitters(t *testing.T) {
	tdb, authority, _ := traceDBRawAsyncFixture(t)
	defer tdb.close()
	ledger := newTraceDBRawAsyncMatchLedger(
		traceDBRawAsyncInventory(traceDBRawAsyncRecords(2000)), authority)
	if ledger.state != "complete_match_only" ||
		ledger.metrics["pairs_matchable"] != 1 {
		t.Fatalf("raw async ledger did not close the exact pair: %s metrics=%+v",
			ledger, ledger.metrics)
	}
	pair, ok := ledger.claim(traceDBCallstackRow{
		OfficialAsyncInterval: true,
		TS:                    1000,
		End:                   2000,
		TID:                   101,
		HeaderTGID:            100,
		TGID:                  100,
		Name:                  "official-async",
		Cookie:                "9",
		StartCPU:              2,
		CPUPlacement:          traceDBSyncSpanCPUPlacementKnown,
	})
	if !ok {
		t.Fatalf("exact raw pair was not claimable: %s metrics=%+v",
			ledger, ledger.metrics)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := pair.publish(sink); err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	for _, row := range sink.rows {
		body.WriteString(row.line)
		body.WriteByte('\n')
	}
	text := body.String()
	if len(sink.rows) != 2 ||
		!strings.Contains(text, "begin-thread-101") ||
		!strings.Contains(text, "finish-thread-301") ||
		!strings.Contains(text, "tracing_mark_write: S|100|official-async|9") ||
		!strings.Contains(text, "tracing_mark_write: F|100|official-async|9") {
		t.Fatalf("raw async emitter/owner separation drifted:\n%s", text)
	}
}

func TestTraceDBCallstackReplacesTypedAsyncOnlyOnUniqueExactRawPair(t *testing.T) {
	for _, test := range []struct {
		name            string
		rawEnd          uint64
		wantRaw         int64
		wantTyped       int64
		wantPhysicalSFP bool
	}{
		{
			name:            "exact pair becomes standard S/F",
			rawEnd:          2000,
			wantRaw:         1,
			wantPhysicalSFP: true,
		},
		{
			name:      "endpoint mismatch remains typed",
			rawEnd:    2001,
			wantTyped: 1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tdb, authority, running := traceDBRawAsyncFixture(t)
			defer tdb.close()
			tdb.sourceNameInventory = traceDBRawAsyncInventory(
				traceDBRawAsyncRecords(test.rawEnd))
			sink, err := newTraceDBRowSink(t.TempDir(), 16)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			syncSpans := newTraceDBTestSyncSpanAuthority(t)
			coverage, err := exportTraceDBCallstack(
				context.Background(), tdb, sink, authority, running, syncSpans)
			if err != nil {
				t.Fatal(err)
			}
			if coverage.Metrics["source_rows_emitted_official_async_raw_pair"] != test.wantRaw ||
				coverage.Metrics["source_rows_emitted_official_async_interval"] != test.wantTyped {
				t.Fatalf("raw/typed replacement accounting drifted: %+v", coverage)
			}
			if test.rawEnd == 2001 &&
				(coverage.Metrics["raw_async_official_intervals_end_mismatch"] != 1 ||
					coverage.Metrics["raw_async_official_intervals_without_exact_raw_pair"] != 1) {
				t.Fatalf("raw async endpoint mismatch was not typed precisely: %+v", coverage)
			}
			var body strings.Builder
			for _, row := range sink.rows {
				body.WriteString(row.line)
				body.WriteByte('\n')
			}
			text := body.String()
			hasPhysical := strings.Contains(text,
				"tracing_mark_write: S|100|official-async|9") &&
				strings.Contains(text,
					"tracing_mark_write: F|100|official-async|9")
			if hasPhysical != test.wantPhysicalSFP ||
				(strings.Contains(text, "# codrax_trace_async_interval/v1") !=
					(test.wantTyped == 1)) {
				t.Fatalf("raw/typed wire selection drifted:\n%s", text)
			}
		})
	}
}

func TestTraceDBRawAsyncClaimExplainsExactEnvelopeMismatch(t *testing.T) {
	tdb, authority, _ := traceDBRawAsyncFixture(t)
	defer tdb.close()
	for _, test := range []struct {
		name       string
		mutate     func(*traceDBCallstackRow)
		wantMetric string
	}{
		{
			name: "identity key",
			mutate: func(row *traceDBCallstackRow) {
				row.Cookie = "10"
			},
			wantMetric: "official_intervals_identity_key_mismatch",
		},
		{
			name: "start emitter tid",
			mutate: func(row *traceDBCallstackRow) {
				row.TID = 102
			},
			wantMetric: "official_intervals_begin_tid_mismatch",
		},
		{
			name: "start emitter tgid",
			mutate: func(row *traceDBCallstackRow) {
				row.HeaderTGID = 101
			},
			wantMetric: "official_intervals_begin_tgid_mismatch",
		},
		{
			name: "start cpu",
			mutate: func(row *traceDBCallstackRow) {
				row.StartCPU = 1
			},
			wantMetric: "official_intervals_begin_cpu_mismatch",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ledger := newTraceDBRawAsyncMatchLedger(
				traceDBRawAsyncInventory(traceDBRawAsyncRecords(2000)), authority)
			row := traceDBCallstackRow{
				OfficialAsyncInterval: true,
				TS:                    1000,
				End:                   2000,
				TID:                   101,
				HeaderTGID:            100,
				TGID:                  100,
				Name:                  "official-async",
				Cookie:                "9",
				StartCPU:              2,
				CPUPlacement:          traceDBSyncSpanCPUPlacementKnown,
			}
			test.mutate(&row)
			if _, ok := ledger.claim(row); ok ||
				ledger.metrics[test.wantMetric] != 1 ||
				ledger.metrics["official_intervals_without_exact_raw_pair"] != 1 {
				t.Fatalf("mismatch classification drifted: metrics=%+v", ledger.metrics)
			}
		})
	}
}

func TestTraceDBRawAsyncDuplicateOpenKeyFailsClosed(t *testing.T) {
	tdb, authority, _ := traceDBRawAsyncFixture(t)
	defer tdb.close()
	rows := traceDBRawAsyncRecords(2000)
	duplicate := rows[0]
	duplicate.PhysicalOrdinal = 2
	duplicate.TimestampNS = 1500
	rows[1].PhysicalOrdinal = 3
	rows = append(rows[:1], duplicate, rows[1])
	ledger := newTraceDBRawAsyncMatchLedger(
		traceDBRawAsyncInventory(rows), authority)
	if ledger.state != "complete_match_only" ||
		ledger.metrics["duplicate_open_starts"] != 1 ||
		ledger.metrics["keys_poisoned"] != 1 ||
		ledger.metrics["pairs_matchable"] != 0 {
		t.Fatalf("duplicate raw async start did not fail closed: state=%s metrics=%+v",
			ledger.state, ledger.metrics)
	}
}
