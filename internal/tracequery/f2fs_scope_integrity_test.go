package tracequery

import "testing"

func TestF2FSSourceScopeSupersedesExactLaneWithinOneSlotBudget(t *testing.T) {
	t.Parallel()
	const source = "/trace/f2fs-scope.systrace"
	verdict := DecodePairingEndpoint("f2fs_direct_IO_enter", f2fsDIOEnter510Body, 40)
	lane, ok := verdict.LaneKey(source)
	if !ok {
		t.Fatalf("valid F2FS endpoint did not expose a lane: %+v", verdict)
	}
	integrity := newDurationPairingIntegrityWithBudget(durationOrderStorage, 1)
	if !integrity.poisonLaneForEvent(lane, source, "f2fs_direct_IO_enter") {
		t.Fatal("initial exact-lane quarantine was not admitted")
	}
	if handled, added := integrity.poisonStorageSourceScope(source, "f2fs_sync_file_enter"); !handled || !added {
		t.Fatalf("source-scope witness did not supersede the exact lane: handled=%t added=%t integrity=%+v", handled, added, integrity)
	}
	if integrity.familyGlobal || integrity.budgetExceeded || integrity.quarantineCount() != 1 || len(integrity.poisonedLanes) != 0 || len(integrity.poisonedSourceScopes) != 1 {
		t.Fatalf("lane-to-scope replacement spuriously overflowed its one-slot budget: %+v", integrity)
	}
	if !integrity.sourcePoisoned(source, "f2fs_write_begin") {
		t.Fatalf("F2FS source scope did not cover sibling exact profiles: %+v", integrity)
	}
	if integrity.sourcePoisoned(source, "scsi_dispatch_cmd_start") {
		t.Fatalf("F2FS source scope leaked into an independent storage profile: %+v", integrity)
	}
}

func TestF2FSTimestampRegressionCarriesEventNameForScopeSupersession(t *testing.T) {
	t.Parallel()
	tracker := newDurationOrderTracker()
	first := Event{Line: 1, Ts: 2, PID: 40, Type: EventFilesystem, Name: "f2fs_direct_IO_enter", FieldText: f2fsDIOEnter510Body}
	if failures := tracker.observeAll(first); len(failures) != 0 {
		t.Fatalf("first valid endpoint unexpectedly failed order audit: %+v", failures)
	}
	second := first
	second.Line, second.Ts = 2, 1
	failures := tracker.observeAll(second)
	if len(failures) != 1 || failures[0].EventName != second.Name || failures[0].LaneKey == "" {
		t.Fatalf("timestamp-regression witness lost exact endpoint identity: %+v", failures)
	}

	const source = "/trace/f2fs-regression.systrace"
	failure := failures[0]
	failure.SourcePath = source
	integrity := newDurationPairingIntegrityWithBudget(durationOrderStorage, 1)
	applyDurationPairingFailure(nil, integrity, failure)
	if len(integrity.poisonedLanes) != 1 || integrity.laneSourceScopes[nextMapKey(integrity.poisonedLanes)] == "" {
		t.Fatalf("regressed exact lane was not associated with its F2FS scope: %+v", integrity)
	}
	applyDurationPairingFailure(nil, integrity, durationOrderViolation{
		Family: durationOrderStorage, EventName: "f2fs_sync_file_enter", SourcePath: source,
	})
	if integrity.familyGlobal || integrity.budgetExceeded || len(integrity.poisonedLanes) != 0 || len(integrity.poisonedSourceScopes) != 1 {
		t.Fatalf("later unknown-key F2FS witness failed to supersede regressed lane: %+v", integrity)
	}
}

func TestF2FSByteDedupDoesNotChangeOtherGenericStorageAccounting(t *testing.T) {
	t.Parallel()
	idx := buildTraceIndex(t, "non-f2fs-byte-account.systrace",
		"io-40 (40) [003] .... 1.000000: scsi_dispatch_cmd_start: tag=-1 dev=8:0 lba=2 len=8 opcode=READ_10\n"+
			"io-40 (40) [003] .... 1.001000: scsi_dispatch_cmd_done: tag=-1 dev=8:0 lba=2 len=8 opcode=READ_10 ret=0\n")
	stats := ComputeWindowStats(idx, Query{TimeStart: .9, TimeEnd: 1.1})
	row := storageLatencyRow(stats.StorageLatencyByLayer, "scsi", "scsi_dispatch_cmd")
	if row == nil || row.PairedCount != 1 || row.Bytes != 16 {
		t.Fatalf("F2FS request-byte dedup changed established SCSI endpoint accounting: row=%+v all=%+v", row, stats.StorageLatencyByLayer)
	}
}

func nextMapKey[V any](items map[string]V) string {
	for key := range items {
		return key
	}
	return ""
}
