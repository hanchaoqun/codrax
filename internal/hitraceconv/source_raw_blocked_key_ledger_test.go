package hitraceconv

import (
	"strings"
	"testing"
)

func traceDBRawBlockedKeyTestAuthority() traceDBSchedulerAuthority {
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 100, Name: "target-process"}
	index.Processes[2] = traceDBProcess{IPID: 2, PID: 200, Name: "header-process"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 101, IPID: 1, Name: "target-thread"}
	index.ByITID[2] = traceDBThread{ITID: 2, TID: 201, IPID: 2, Name: "header-thread"}
	buildTraceDBThreadSecondaryIndexes(&index)
	return newTraceDBSchedulerAuthority(index, traceDBLifecycleCollection{
		CreationComplete: true,
		TerminalComplete: true,
		ActivityComplete: true,
	})
}

func TestTraceDBRawBlockedKeyCoverageWithholdsOverlapCohort(t *testing.T) {
	rawRows := []traceDBRawBlockedRecord{
		{TimestampNS: 100, CPU: 1, HeaderPID: 201, TargetTID: 101, IOWait: 1, CallerRaw: 0x123},
		{TimestampNS: 101, CPU: 2, HeaderPID: 201, TargetTID: 101, IOWait: 0, CallerRaw: 0x456},
		{TimestampNS: 102, CPU: 3, HeaderPID: 201, TargetTID: 101, IOWait: 0, CallerRaw: 0x456},
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
	got := traceDBRawBlockedKeyCoverage(inventory, dbRows, traceDBRawBlockedKeyTestAuthority())
	if !got.Found || got.RowsRead != 4 || got.RowsEmitted != 0 ||
		got.Metadata["ledger_state"] != "exact_content_multiset_subset" ||
		got.Metrics["raw_overlap_key_cohorts"] != 1 ||
		got.Metrics["raw_rows_withheld_overlap_cohort"] != 1 ||
		got.Metrics["raw_absent_db_key_cohorts"] != 1 ||
		got.Metrics["raw_rows_absent_db_cohort"] != 2 ||
		got.Metrics["raw_rows_absent_db_identity_admitted"] != 2 ||
		got.Metadata["raw_key_multiset_sha256"] == "" ||
		got.Metadata["db_key_multiset_sha256"] == "" {
		t.Fatalf("blocked key subset ledger mismatch: %+v", got)
	}
}

func TestTraceDBRawBlockedKeyCoverageFailsClosedForNamespaceAndMultisetMismatch(t *testing.T) {
	t.Run("unresolved public tid is not rewritten as host identity", func(t *testing.T) {
		inventory := &traceDBSourceNameInventory{
			RawDecode: TraceDBCoverage{
				Found: true,
				Metrics: map[string]int64{
					"target_sched_blocked_reason_body_admitted": 1,
				},
				Metadata: map[string]string{"decode_state": "strict_target_ledger_complete"},
			},
			RawBlocked: []traceDBRawBlockedRecord{{
				TimestampNS: 100, CPU: 1, HeaderPID: 201,
				TargetTID: 999, IOWait: 1, CallerRaw: 0x123,
			}},
		}
		got := traceDBRawBlockedKeyCoverage(inventory, nil, traceDBRawBlockedKeyTestAuthority())
		if got.Metadata["ledger_state"] != "exact_content_multiset_subset" ||
			got.Metrics["raw_rows_absent_db_identity_admitted"] != 0 {
			t.Fatalf("unresolved namespace-shaped TID gained identity: %+v", got)
		}
		foundTypedReject := false
		for metric, count := range got.Metrics {
			if strings.HasPrefix(metric, "raw_rows_absent_db_identity_rejected_target_") && count == 1 {
				foundTypedReject = true
			}
		}
		if !foundTypedReject {
			t.Fatalf("unresolved public TID rejection was not typed: %+v", got)
		}
	})

	t.Run("DB cohort absent from raw withdraws the ledger", func(t *testing.T) {
		inventory := &traceDBSourceNameInventory{
			RawDecode: TraceDBCoverage{
				Found: true,
				Metrics: map[string]int64{
					"target_sched_blocked_reason_body_admitted": 1,
				},
				Metadata: map[string]string{"decode_state": "strict_target_ledger_complete"},
			},
			RawBlocked: []traceDBRawBlockedRecord{{
				TimestampNS: 100, CPU: 1, HeaderPID: 201,
				TargetTID: 101, IOWait: 1, CallerRaw: 0x123,
			}},
		}
		dbRows := []traceDBPreparedBlockedReason{{
			Row:    traceDBBlockedStateRow{TID: 101},
			IOWait: 0, Caller: "unknown", CallerRaw: "0x999", CallerQuality: "opaque",
		}}
		got := traceDBRawBlockedKeyCoverage(inventory, dbRows, traceDBRawBlockedKeyTestAuthority())
		if got.Metadata["ledger_state"] != "exact_content_multiset_subset_mismatch" ||
			got.Metrics["db_key_cohorts_absent_raw"] != 1 ||
			got.Metadata["publication_authority"] != "withheld_diagnostic_only" {
			t.Fatalf("DB/raw multiset mismatch did not withdraw authority: %+v", got)
		}
	})
}

func TestTraceDBRawBlockedComparableKeyKeepsCallerProfilesSeparate(t *testing.T) {
	rawSymbol := traceDBRawBlockedRecord{
		TargetTID: 101, IOWait: 1, CallerRaw: 0x123,
		Caller: "schedule+0x1/0x2[kernel]", CallerSymbolized: true,
	}
	symbolKey, symbolOK := traceDBRawBlockedComparableKey(rawSymbol)
	rawKey, rawOK := traceDBRawBlockedComparableKey(traceDBRawBlockedRecord{
		TargetTID: 101, IOWait: 1, CallerRaw: 0x123,
	})
	dbSymbolKey, dbSymbolOK := traceDBDBBlockedComparableKey(traceDBPreparedBlockedReason{
		Row: traceDBBlockedStateRow{TID: 101}, IOWait: 1,
		Caller: "schedule+0x1/0x2[kernel]", CallerQuality: "symbolized",
	})
	if !symbolOK || !rawOK || !dbSymbolOK || symbolKey != dbSymbolKey || symbolKey == rawKey {
		t.Fatalf("caller profiles collapsed or failed parity: symbol=%+v raw=%+v db=%+v",
			symbolKey, rawKey, dbSymbolKey)
	}
}
