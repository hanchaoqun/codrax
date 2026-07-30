package hitraceconv

import (
	"strings"
	"testing"
)

func traceDBRawSchedulerCPUFallbackAuthority() traceDBSchedulerAuthority {
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 100, Name: "proc-a"}
	index.Processes[2] = traceDBProcess{IPID: 2, PID: 200, Name: "proc-b"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 42, IPID: 1, Name: "thread-a"}
	index.ByITID[2] = traceDBThread{ITID: 2, TID: 77, IPID: 2, Name: "thread-b"}
	buildTraceDBThreadSecondaryIndexes(&index)
	return newTraceDBSchedulerAuthority(index, traceDBLifecycleCollection{
		CreationComplete: true,
		TerminalComplete: true,
		ActivityComplete: true,
	})
}

func traceDBRawSchedulerCPUFallbackInventory(
	rows []traceDBRawSchedSwitchLiteRecord,
) *traceDBSourceNameInventory {
	return &traceDBSourceNameInventory{
		RawDecode: TraceDBCoverage{
			Role: "diagnostic_ledger",
			Metadata: map[string]string{
				"decode_state": "strict_target_ledger_complete",
			},
			Metrics: map[string]int64{
				"target_sched_switch_lite_records":       int64(len(rows)),
				"target_sched_switch_lite_body_admitted": int64(len(rows)),
			},
		},
		RawSwitchLite: append([]traceDBRawSchedSwitchLiteRecord(nil), rows...),
	}
}

func traceDBRawSchedulerCPUFallbackRows() []traceDBRawSchedSwitchLiteRecord {
	return []traceDBRawSchedSwitchLiteRecord{
		{
			TimestampNS: 100, CPU: 4, PrevTID: 0, PrevPriority: 10,
			PrevState: 1, NextTID: 42, NextPriority: 20,
		},
		{
			TimestampNS: 200, CPU: 4, PrevTID: 42, PrevPriority: 20,
			PrevState: 1, NextTID: 77, NextPriority: 30,
		},
		{
			TimestampNS: 300, CPU: 4, PrevTID: 77, PrevPriority: 30,
			PrevState: 1, NextTID: 0, NextPriority: 10,
		},
	}
}

func TestTraceDBRawSchedulerCPUFallbackRecoversUnknownAndTaintedDBPoints(t *testing.T) {
	authority := traceDBRawSchedulerCPUFallbackAuthority()
	fallback := newTraceDBRawSchedulerCPUFallback(
		traceDBRawSchedulerCPUFallbackInventory(
			traceDBRawSchedulerCPUFallbackRows()),
		authority)
	if !fallback.enabled || fallback.coverage.RowsEmitted != 2 ||
		fallback.coverage.Metadata["authority_state"] !=
			"complete_unique_exact_half_open_intervals" ||
		fallback.coverage.Metrics["canonical_itid_lanes"] != 2 {
		t.Fatalf("raw scheduler CPU authority did not close: %+v", fallback.coverage)
	}
	running := newTraceDBSchedulerRunningIndex(
		authority,
		map[int64][]traceDBRunningInterval{},
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{1: true}},
		nil).withRawSchedulerCPUFallback(fallback)
	if cpu, status := running.lookupCPUAt(1, 150); status != traceDBSchedulerRunningKnown || cpu != 4 {
		t.Fatalf("source-tainted DB point not recovered: cpu=%d status=%d", cpu, status)
	}
	if cpu, status := running.lookupCPUAt(2, 250); status != traceDBSchedulerRunningKnown || cpu != 4 {
		t.Fatalf("unknown DB point not recovered: cpu=%d status=%d", cpu, status)
	}
	if _, status := running.lookupCPUAt(1, 200); status != traceDBSchedulerRunningSourceTainted {
		t.Fatalf("half-open raw interval leaked through switch boundary: status=%d", status)
	}
}

func TestTraceDBRawSchedulerCPUFallbackDBAgreementAndConflict(t *testing.T) {
	authority := traceDBRawSchedulerCPUFallbackAuthority()
	fallback := newTraceDBRawSchedulerCPUFallback(
		traceDBRawSchedulerCPUFallbackInventory(
			traceDBRawSchedulerCPUFallbackRows()),
		authority)
	agree := newTraceDBSchedulerRunningIndex(
		authority,
		map[int64][]traceDBRunningInterval{
			1: {{Start: 100, End: 200, CPU: 4, PrefixMaxEnd: 200}},
		},
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}},
		nil).withRawSchedulerCPUFallback(fallback)
	if cpu, status := agree.lookupCPUAt(1, 150); status != traceDBSchedulerRunningKnown || cpu != 4 {
		t.Fatalf("agreeing DB/raw CPU lost: cpu=%d status=%d", cpu, status)
	}
	conflict := newTraceDBSchedulerRunningIndex(
		authority,
		map[int64][]traceDBRunningInterval{
			1: {{Start: 100, End: 200, CPU: 3, PrefixMaxEnd: 200}},
		},
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}},
		nil).withRawSchedulerCPUFallback(fallback)
	if _, status := conflict.lookupCPUAt(1, 150); status != traceDBSchedulerRunningSourceTainted {
		t.Fatalf("contradictory DB/raw CPU did not fail closed: status=%d", status)
	}

	lifecycleRejected := agree
	lifecycleRejected.lifecycleRejectedITID = map[int64]bool{1: true}
	if _, status := lifecycleRejected.lookupCPUAt(1, 150); status != traceDBSchedulerRunningLifecycleRejected {
		t.Fatalf("raw fallback bypassed lifecycle rejection: status=%d", status)
	}
}

func TestTraceDBRawSchedulerCPUFallbackRequiresCompleteZeroRejectFamily(t *testing.T) {
	authority := traceDBRawSchedulerCPUFallbackAuthority()
	inventory := traceDBRawSchedulerCPUFallbackInventory(
		traceDBRawSchedulerCPUFallbackRows())
	inventory.RawDecode.Metrics["target_sched_switch_lite_body_admitted"] = 2
	inventory.RawDecode.Metrics["target_sched_switch_lite_body_rejected"] = 1
	fallback := newTraceDBRawSchedulerCPUFallback(inventory, authority)
	if fallback.enabled || fallback.coverage.RowsEmitted != 0 ||
		fallback.coverage.Metadata["authority_state"] !=
			"withheld_incomplete_physical_event_family" ||
		!strings.Contains(fallback.coverage.Skipped, "rejected=1") {
		t.Fatalf("incomplete raw scheduler family failed open: %+v", fallback.coverage)
	}
}

func TestTraceDBRawSchedulerCPUFallbackFencesDiscontinuousBoundary(t *testing.T) {
	authority := traceDBRawSchedulerCPUFallbackAuthority()
	rows := traceDBRawSchedulerCPUFallbackRows()
	rows[1].PrevTID = 77
	fallback := newTraceDBRawSchedulerCPUFallback(
		traceDBRawSchedulerCPUFallbackInventory(rows), authority)
	if !fallback.enabled || fallback.coverage.RowsEmitted != 1 ||
		fallback.coverage.Metrics["candidate_intervals_withheld_tid_discontinuity"] != 1 {
		t.Fatalf("raw TID discontinuity was not localized as a fence: %+v",
			fallback.coverage)
	}
	if _, ok := traceDBKnownCPUAt(fallback.intervals, 1, 150); ok {
		t.Fatal("discontinuous raw boundary minted a CPU interval")
	}
	if cpu, ok := traceDBKnownCPUAt(fallback.intervals, 2, 250); !ok || cpu != 4 {
		t.Fatalf("independent post-fence interval was lost: cpu=%d ok=%t", cpu, ok)
	}
}
