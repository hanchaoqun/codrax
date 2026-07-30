package hitraceconv

import (
	"os"
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
		nil).
		withRawSchedulerCPUFallback(fallback).
		withRawSchedulerCPUConsumer(traceDBRawSchedulerCPUConsumerCallstack)
	if cpu, status := running.lookupCPUAt(1, 150); status != traceDBSchedulerRunningKnown || cpu != 4 {
		t.Fatalf("source-tainted DB point not recovered: cpu=%d status=%d", cpu, status)
	}
	if cpu, status := running.lookupCPUAt(2, 250); status != traceDBSchedulerRunningKnown || cpu != 4 {
		t.Fatalf("unknown DB point not recovered: cpu=%d status=%d", cpu, status)
	}
	if _, status := running.lookupCPUAt(1, 200); status != traceDBSchedulerRunningSourceTainted {
		t.Fatalf("half-open raw interval leaked through switch boundary: status=%d", status)
	}
	coverage := fallback.finalCoverage()
	if coverage.Metrics["lookup_calls_total"] != 3 ||
		coverage.Metrics["lookup_callstack_db_source_tainted_raw_known"] != 1 ||
		coverage.Metrics["lookup_callstack_db_unknown_raw_known"] != 1 ||
		coverage.Metrics["lookup_callstack_db_source_tainted_raw_miss_after_last_interval"] != 1 ||
		coverage.Metadata["lookup_census_state"] !=
			"complete_after_callstack_and_frame_consumers" ||
		!strings.Contains(
			coverage.Metadata["lookup_callstack_db_source_tainted_raw_miss_after_last_interval_witnesses"],
			"itid=1/tid=42/ts_ns=200") {
		t.Fatalf("raw scheduler CPU lookup census did not explain recovery/miss: %+v",
			coverage)
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
		nil).
		withRawSchedulerCPUFallback(fallback).
		withRawSchedulerCPUConsumer(traceDBRawSchedulerCPUConsumerFrame)
	if cpu, status := agree.lookupCPUAt(1, 150); status != traceDBSchedulerRunningKnown || cpu != 4 {
		t.Fatalf("agreeing DB/raw CPU lost: cpu=%d status=%d", cpu, status)
	}
	conflict := newTraceDBSchedulerRunningIndex(
		authority,
		map[int64][]traceDBRunningInterval{
			1: {{Start: 100, End: 200, CPU: 3, PrefixMaxEnd: 200}},
		},
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}},
		nil).
		withRawSchedulerCPUFallback(fallback).
		withRawSchedulerCPUConsumer(traceDBRawSchedulerCPUConsumerFrame)
	if _, status := conflict.lookupCPUAt(1, 150); status != traceDBSchedulerRunningSourceTainted {
		t.Fatalf("contradictory DB/raw CPU did not fail closed: status=%d", status)
	}

	lifecycleRejected := agree
	lifecycleRejected.lifecycleRejectedITID = map[int64]bool{1: true}
	if _, status := lifecycleRejected.lookupCPUAt(1, 150); status != traceDBSchedulerRunningLifecycleRejected {
		t.Fatalf("raw fallback bypassed lifecycle rejection: status=%d", status)
	}
	coverage := fallback.finalCoverage()
	if coverage.Metrics["lookup_calls_total"] != 3 ||
		coverage.Metrics["lookup_frame_db_known_raw_known_agree"] != 1 ||
		coverage.Metrics["lookup_frame_db_known_raw_known_conflict"] != 1 ||
		coverage.Metrics["lookup_frame_db_lifecycle_rejected_raw_not_consulted"] != 1 {
		t.Fatalf("raw scheduler CPU agreement/conflict census mismatch: %+v",
			coverage)
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

func TestTraceDBRawSchedulerCPULookupClassifiesMissGeometry(t *testing.T) {
	intervals := map[int64][]traceDBRunningInterval{
		1: {
			{Start: 100, End: 150, CPU: 1, PrefixMaxEnd: 150},
			{Start: 200, End: 250, CPU: 1, PrefixMaxEnd: 250},
		},
		2: {
			{Start: 100, End: 200, CPU: 1, PrefixMaxEnd: 200},
			{Start: 150, End: 250, CPU: 2, PrefixMaxEnd: 250},
		},
	}
	for _, test := range []struct {
		name   string
		itid   int64
		ts     int64
		status traceDBRawSchedulerCPURawLookupStatus
	}{
		{name: "lane absent", itid: 9, ts: 175, status: traceDBRawSchedulerCPURawMissLaneAbsent},
		{name: "before first", itid: 1, ts: 99, status: traceDBRawSchedulerCPURawMissBeforeFirst},
		{name: "gap", itid: 1, ts: 175, status: traceDBRawSchedulerCPURawMissGap},
		{name: "after last", itid: 1, ts: 250, status: traceDBRawSchedulerCPURawMissAfterLast},
		{name: "overlap conflict", itid: 2, ts: 175, status: traceDBRawSchedulerCPURawMissOverlapConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, status := traceDBRawSchedulerCPUAt(
				intervals, test.itid, test.ts); status != test.status {
				t.Fatalf("raw miss classification=%d, want %d", status, test.status)
			}
		})
	}
}

func TestTraceDBRawSchedulerCPULookupWitnessesAreBounded(t *testing.T) {
	authority := traceDBRawSchedulerCPUFallbackAuthority()
	fallback := newTraceDBRawSchedulerCPUFallback(
		traceDBRawSchedulerCPUFallbackInventory(
			traceDBRawSchedulerCPUFallbackRows()),
		authority)
	running := newTraceDBSchedulerRunningIndex(
		authority, map[int64][]traceDBRunningInterval{},
		traceDBRunningIntegrity{TaintedITIDs: map[int64]bool{}}, nil).
		withRawSchedulerCPUFallback(fallback).
		withRawSchedulerCPUConsumer(traceDBRawSchedulerCPUConsumerCallstack)
	for index := int64(0); index < 6; index++ {
		if _, status := running.lookupCPUAt(99, 150+index); status !=
			traceDBSchedulerRunningUnknown {
			t.Fatalf("absent raw lane unexpectedly recovered: status=%d", status)
		}
	}
	coverage := fallback.finalCoverage()
	key := "lookup_callstack_db_unknown_raw_miss_lane_absent"
	if coverage.Metrics[key] != 6 ||
		coverage.Metrics[key+"_witnesses_emitted"] !=
			traceDBRawSchedulerCPULookupWitnessCap ||
		coverage.Metrics[key+"_witnesses_omitted"] != 2 ||
		strings.Count(coverage.Metadata[key+"_witnesses"], "itid=") !=
			traceDBRawSchedulerCPULookupWitnessCap {
		t.Fatalf("raw CPU lookup witness bound mismatch: %+v", coverage)
	}
}

func TestTraceDBRawSchedulerCPULookupCensusProductionWiring(t *testing.T) {
	body, err := os.ReadFile("streamerdb_export_extended.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	build := strings.Index(text, "newTraceDBRawSchedulerCPUFallback(")
	callstack := strings.Index(text, "exportTraceDBCallstack(")
	frame := strings.Index(text, "exportTraceDBFrameSliceWithRows(")
	finalize := strings.Index(text, "rawSchedulerCPU.finalCoverage()")
	if build < 0 || callstack < build || frame < callstack || finalize < frame {
		t.Fatalf("raw CPU lookup census is not finalized after both authorized consumers: build=%d callstack=%d frame=%d finalize=%d",
			build, callstack, frame, finalize)
	}
	if strings.Count(text, "traceDBRawSchedulerCPUConsumerCallstack") != 1 ||
		strings.Count(text, "traceDBRawSchedulerCPUConsumerFrame") != 1 {
		t.Fatal("raw CPU lookup census consumer scopes are not uniquely wired")
	}
}
