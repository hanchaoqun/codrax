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
		"CREATE TABLE thread (id, itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 1, 100, 1, 'logical-owner', 0, 1, 1)",
		"INSERT INTO thread VALUES (2, 2, 101, 1, 'begin-thread', 0, 0, 1)",
		"INSERT INTO thread VALUES (3, 3, 301, 2, 'finish-thread', 0, 0, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (2, 0, 3000, 2, 'Running')",
		"CREATE TABLE callstack (id, ts, dur, callid, name, flag, cookie, chainId, depth, child_callid)",
		"INSERT INTO callstack VALUES (1, 1000, 1000, 1, 'official-async', NULL, 9, NULL, 0, 2)",
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

func TestTraceDBRawAsyncLedgerJoinsDistinctNamespacePayloadPID(t *testing.T) {
	tdb, authority, _ := traceDBRawAsyncFixture(t)
	defer tdb.close()
	rows := traceDBRawAsyncRecords(2000)
	for index := range rows {
		rows[index].PayloadPID = 37722
		rows[index].Buffer = strings.Replace(rows[index].Buffer, "|100|", "|37722|", 1)
	}
	ledger := newTraceDBRawAsyncMatchLedger(
		traceDBRawAsyncInventory(rows), authority)
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
	if !ok || pair == nil ||
		ledger.metrics["official_intervals_namespace_payload_pid_joined"] != 1 ||
		ledger.metrics["pairs_claimed"] != 1 {
		t.Fatalf("distinct namespace payload PID was not joined by exact physical envelope: metrics=%+v",
			ledger.metrics)
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	if err := pair.publish(sink); err != nil {
		t.Fatal(err)
	}
	text := sink.rows[0].line + "\n" + sink.rows[1].line
	if !strings.Contains(text, "S|37722|official-async|9") ||
		!strings.Contains(text, "F|37722|official-async|9") ||
		!strings.Contains(text, "begin-thread-101") ||
		!strings.Contains(text, "(  100)") {
		t.Fatalf("raw namespace PID or host emitter envelope was rewritten:\n%s", text)
	}
}

func TestTraceDBRawAsyncLedgerAmbiguousNamespacePayloadPIDFailsClosed(t *testing.T) {
	tdb, authority, _ := traceDBRawAsyncFixture(t)
	defer tdb.close()
	first := traceDBRawAsyncRecords(2000)
	second := traceDBRawAsyncRecords(2000)
	for index := range first {
		first[index].PayloadPID = 37722
		first[index].Buffer = strings.Replace(first[index].Buffer, "|100|", "|37722|", 1)
	}
	for index := range second {
		second[index].PhysicalOrdinal += 2
		second[index].PayloadPID = 47722
		second[index].Buffer = strings.Replace(second[index].Buffer, "|100|", "|47722|", 1)
	}
	ledger := newTraceDBRawAsyncMatchLedger(
		traceDBRawAsyncInventory(append(first, second...)), authority)
	_, ok := ledger.claim(traceDBCallstackRow{
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
	if ok ||
		ledger.metrics["official_intervals_ambiguous_exact_raw_pair"] != 1 ||
		ledger.metrics["official_intervals_without_exact_raw_pair"] != 1 {
		t.Fatalf("ambiguous namespace payload candidates did not fail closed: %+v",
			ledger.metrics)
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
			if coverage.Metrics["source_rows_official_async_child_emitter_resolved"] != 1 {
				t.Fatalf("official child_callid emitter was not selected: %+v", coverage)
			}
			if test.rawEnd == 2001 &&
				(coverage.Metrics["raw_async_official_intervals_end_mismatch"] != 1 ||
					coverage.Metrics["raw_async_official_intervals_without_exact_raw_pair"] != 1 ||
					!strings.Contains(coverage.Metadata["raw_async_mismatch_witnesses"],
						"class=end_mismatch")) {
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

func TestTraceDBCallstackOfficialChildEmitterPreservesNamespaceOwner(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (id, ipid, pid, name)",
		"INSERT INTO process VALUES (1, 1, 17267, 'host-process')",
		"INSERT INTO process VALUES (2, 2, 37722, 'namespace-owner')",
		"CREATE TABLE thread (id, itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 1, 17267, 1, 'host-emitter', 0, 1, 1)",
		"INSERT INTO thread VALUES (2, 2, 37722, 2, 'logical-owner', 0, 0, 0)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (1, 0, 3000, 4, 'Running')",
		"CREATE TABLE callstack (id, ts, dur, callid, name, flag, cookie, chainId, depth, child_callid)",
		"INSERT INTO callstack VALUES (1, 1000, 1000, 2, 'namespace-async', NULL, 11, NULL, 0, 1)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	raw := []traceDBRawMarkerRecord{
		{
			PhysicalOrdinal: 1, TimestampNS: 1000, CPU: 4, HeaderPID: 17267,
			Buffer: "S|37722|namespace-async|11", Action: "S", PayloadPID: 37722,
			Name: "namespace-async", Value: "11", Admitted: true,
		},
		{
			PhysicalOrdinal: 2, TimestampNS: 2000, CPU: 5, HeaderPID: 17267,
			Buffer: "F|37722|namespace-async|11", Action: "F", PayloadPID: 37722,
			Name: "namespace-async", Value: "11", Admitted: true,
		},
	}
	tdb.sourceNameInventory = traceDBRawAsyncInventory(raw)
	identities, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	intervals, integrity, _, err := tdb.loadRunningIntervals(context.Background(), identities)
	if err != nil {
		t.Fatal(err)
	}
	authority := traceDBSchedulerAuthority{
		identities: identities, lifecycle: traceDBLifecycleIndex{},
		initialized: true, complete: true,
	}
	running := newTraceDBSchedulerRunningIndex(authority, intervals, integrity, nil)
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := exportTraceDBCallstack(context.Background(), tdb, sink, authority, running,
		newTraceDBTestSyncSpanAuthority(t))
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Metrics["source_rows_emitted_official_async_raw_pair"] != 1 ||
		coverage.Metrics["source_rows_official_async_child_emitter_resolved"] != 1 {
		t.Fatalf("namespace async child emitter was not closed: %+v", coverage)
	}
	var body strings.Builder
	for _, row := range sink.rows {
		body.WriteString(row.line)
		body.WriteByte('\n')
	}
	text := body.String()
	if strings.Count(text, "tracing_mark_write:") != 2 ||
		!strings.Contains(text, "S|37722|namespace-async|11") ||
		!strings.Contains(text, "F|37722|namespace-async|11") ||
		!strings.Contains(text, "host-emitter-17267 (17267)") {
		t.Fatalf("namespace owner replaced the physical header or was lost:\n%s", text)
	}
}

func TestTraceDBCallstackOfficialChildEmitterMissingFailsClosed(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1, 100, 'process')",
		"CREATE TABLE thread (id, itid, tid, ipid, name, start_ts, is_main_thread, switch_count)",
		"INSERT INTO thread VALUES (1, 1, 100, 1, 'owner', 0, 1, 1)",
		"CREATE TABLE thread_state (itid, ts, dur, cpu, state)",
		"INSERT INTO thread_state VALUES (1, 0, 3000, 2, 'Running')",
		"CREATE TABLE callstack (id, ts, dur, callid, name, flag, cookie, chainId, depth, child_callid)",
		"INSERT INTO callstack VALUES (1, 1000, 1000, 1, 'missing-child', NULL, 9, NULL, 0, NULL)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	identities, _, err := tdb.loadThreadIndex(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	intervals, integrity, _, err := tdb.loadRunningIntervals(context.Background(), identities)
	if err != nil {
		t.Fatal(err)
	}
	authority := traceDBSchedulerAuthority{
		identities: identities, lifecycle: traceDBLifecycleIndex{},
		initialized: true, complete: true,
	}
	sink, err := newTraceDBRowSink(t.TempDir(), 4)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := exportTraceDBCallstack(context.Background(), tdb, sink, authority,
		newTraceDBSchedulerRunningIndex(authority, intervals, integrity, nil),
		newTraceDBTestSyncSpanAuthority(t))
	if err != nil {
		t.Fatal(err)
	}
	if coverage.RowsEmitted != 0 ||
		!strings.Contains(coverage.Skipped, "async_child_missing_emitter_identity=1") ||
		coverage.Metrics["source_rows_rejected_official_async_shape"] != 1 {
		t.Fatalf("missing official child emitter did not fail closed: %+v", coverage)
	}
}

func TestTraceDBRawAsyncClaimExplainsExactEnvelopeMismatch(t *testing.T) {
	tdb, authority, _ := traceDBRawAsyncFixture(t)
	defer tdb.close()
	for _, test := range []struct {
		name       string
		mutate     func(*traceDBCallstackRow)
		wantMetric string
		wantClass  string
	}{
		{
			name: "cookie",
			mutate: func(row *traceDBCallstackRow) {
				row.Cookie = "10"
			},
			wantMetric: "official_intervals_cookie_mismatch",
			wantClass:  "cookie_mismatch",
		},
		{
			name: "name",
			mutate: func(row *traceDBCallstackRow) {
				row.Name = "different-name"
			},
			wantMetric: "official_intervals_name_mismatch",
			wantClass:  "name_mismatch",
		},
		{
			name: "name and cookie",
			mutate: func(row *traceDBCallstackRow) {
				row.Name = "different-name"
				row.Cookie = "10"
			},
			wantMetric: "official_intervals_name_cookie_mismatch",
			wantClass:  "name_cookie_mismatch",
		},
		{
			name: "start emitter tid",
			mutate: func(row *traceDBCallstackRow) {
				row.TID = 102
			},
			wantMetric: "official_intervals_begin_tid_mismatch",
			wantClass:  "begin_tid_mismatch",
		},
		{
			name: "start emitter tgid",
			mutate: func(row *traceDBCallstackRow) {
				row.HeaderTGID = 101
			},
			wantMetric: "official_intervals_begin_tgid_mismatch",
			wantClass:  "begin_tgid_mismatch",
		},
		{
			name: "start cpu",
			mutate: func(row *traceDBCallstackRow) {
				row.StartCPU = 1
			},
			wantMetric: "official_intervals_begin_cpu_mismatch",
			wantClass:  "begin_cpu_mismatch",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ledger := newTraceDBRawAsyncMatchLedger(
				traceDBRawAsyncInventory(traceDBRawAsyncRecords(2000)), authority)
			row := traceDBCallstackRow{
				ID:                    1,
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
			coverage := TraceDBCoverage{
				FieldSources: map[string]string{}, Metadata: map[string]string{},
			}
			ledger.applyCoverage(&coverage)
			if coverage.Metrics["raw_async_mismatch_witnesses_emitted"] != 1 ||
				coverage.Metadata["raw_async_mismatch_census"] != "complete" ||
				!strings.Contains(coverage.Metadata["raw_async_mismatch_witnesses"],
					"row_id=1/class="+test.wantClass) {
				t.Fatalf("mismatch witness drifted: %+v", coverage)
			}
		})
	}
}

func TestTraceDBRawAsyncMismatchWitnessesAreBounded(t *testing.T) {
	tdb, authority, _ := traceDBRawAsyncFixture(t)
	defer tdb.close()
	ledger := newTraceDBRawAsyncMatchLedger(
		traceDBRawAsyncInventory(traceDBRawAsyncRecords(2000)), authority)
	for index := 0; index < traceDBRawAsyncMismatchWitnessCap+2; index++ {
		if _, ok := ledger.claim(traceDBCallstackRow{
			ID:                    int64(index + 1),
			OfficialAsyncInterval: true,
			TS:                    1000,
			End:                   2000,
			TID:                   101,
			HeaderTGID:            100,
			TGID:                  100,
			Name:                  "official-async",
			Cookie:                "10",
			StartCPU:              2,
			CPUPlacement:          traceDBSyncSpanCPUPlacementKnown,
		}); ok {
			t.Fatal("mismatched cookie unexpectedly claimed a raw pair")
		}
	}
	coverage := TraceDBCoverage{
		FieldSources: map[string]string{}, Metadata: map[string]string{},
	}
	ledger.applyCoverage(&coverage)
	if coverage.Metrics["raw_async_mismatch_witnesses_emitted"] !=
		traceDBRawAsyncMismatchWitnessCap ||
		coverage.Metrics["raw_async_mismatch_witnesses_omitted"] != 2 ||
		strings.Count(coverage.Metadata["raw_async_mismatch_witnesses"],
			"class=cookie_mismatch") != traceDBRawAsyncMismatchWitnessCap {
		t.Fatalf("bounded raw async mismatch witnesses drifted: %+v", coverage)
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
