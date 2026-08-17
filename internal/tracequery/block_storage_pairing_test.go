package tracequery

import "testing"

func TestBlockPairingExactEndpointFamiliesNeverCross(t *testing.T) {
	idx := buildTraceIndex(t, "block-family.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
 io-40 (40) [003] .... 1.001000: block_bio_complete: 8,0 R 123 + 8 [0]
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.002})
	if len(stats.IOLatencies) != 0 {
		t.Fatalf("rq issue cross-paired with bio completion: %+v", stats.IOLatencies)
	}
	if !containsSubstring(stats.Caveats, "block_io_pairing_unpaired=true") {
		t.Fatalf("family mismatch was not disclosed as unpaired: %+v", stats.Caveats)
	}
}

func TestBlockInsertAndGetRQAreInventoryNotLatencyStarts(t *testing.T) {
	idx := buildTraceIndex(t, "block-inventory.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_insert: 8,0 R 4096 () 123 + 8 [io]
 io-40 (40) [003] .... 1.001000: block_getrq: 8,0 R 4096 () 123 + 8 [io]
 io-40 (40) [003] .... 1.002000: block_rq_complete: 8,0 R () 123 + 8 [0]
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.003})
	if stats.BlockIssueCount != 2 {
		t.Fatalf("insert/getrq inventory disappeared: block_issue_count=%d", stats.BlockIssueCount)
	}
	if len(stats.IOLatencies) != 0 {
		t.Fatalf("insert/getrq opened a request latency lane: %+v", stats.IOLatencies)
	}
	if containsSubstring(stats.Caveats, "duration_pairing_fail_closed=true") {
		t.Fatalf("inventory-only endpoints poisoned elapsed duration audit: %+v", stats.Caveats)
	}
}

func TestBlockSectorZeroIsAValidExplicitIdentity(t *testing.T) {
	idx := buildTraceIndex(t, "block-sector-zero.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 0 + 8 [io]
 io-40 (40) [003] .... 1.002000: block_rq_complete: 8,0 R () 0 + 8 [0]
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.003})
	if len(stats.IOLatencies) != 1 || stats.IOLatencies[0].Sector != 0 || !near(stats.IOLatencies[0].DurationMs, 2, 0.001) {
		t.Fatalf("explicit sector zero failed to pair: %+v caveats=%+v", stats.IOLatencies, stats.Caveats)
	}
}

func TestBlockZeroLengthFlushIsARealPairButZeroLengthReadIsInvalid(t *testing.T) {
	idx := buildTraceIndex(t, "block-flush-zero.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 12,80 FS 0 () 0 + 0 [io]
 irq-2 (2) [003] .... 1.001226: block_rq_complete: 12,80 FS () 0 + 0 [0]
 io-40 (40) [003] .... 1.002000: block_rq_issue: 12,80 R 4096 () 1 + 0 [io]
 irq-2 (2) [003] .... 1.003000: block_rq_complete: 12,80 R () 1 + 0 [0]
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.004})
	if len(stats.IOLatencies) != 1 {
		t.Fatalf("expected only the exact zero-length flush pair: %+v caveats=%+v", stats.IOLatencies, stats.Caveats)
	}
	flush := stats.IOLatencies[0]
	if flush.Op != "FS" || flush.Sector != 0 || flush.Len != 0 || !near(flush.DurationMs, 1.226, 0.001) {
		t.Fatalf("production FS 0+0 latency was not preserved: %+v", flush)
	}
	if !containsSubstring(stats.Caveats, "block_io_pairing_identity_invalid=true") {
		t.Fatalf("ordinary R 1+0 must remain invalid and fail loud: %+v", stats.Caveats)
	}
}

func TestBlockOverlappingZeroLengthFlushCohortIsNeverFIFOGuessed(t *testing.T) {
	idx := buildTraceIndex(t, "block-flush-ambiguous.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 12,80 FS 0 () 0 + 0 [io]
 io-41 (41) [003] .... 1.001000: block_rq_issue: 12,80 FS 0 () 0 + 0 [io]
 irq-2 (2) [003] .... 1.002000: block_rq_complete: 12,80 FS () 0 + 0 [0]
 irq-2 (2) [003] .... 1.003000: block_rq_complete: 12,80 FS () 0 + 0 [0]
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.004})
	row := storageLatencyRow(stats.StorageLatencyByLayer, "block", blockEndpointFamilyRQ)
	if len(stats.IOLatencies) != 0 || row == nil || row.AmbiguousCohortCount != 1 || row.PairingSuppressedCount != 2 {
		t.Fatalf("overlapping FS 0+0 requests must suppress the whole cohort: latencies=%+v rows=%+v", stats.IOLatencies, stats.StorageLatencyByLayer)
	}
}

func TestBlockConcurrentCoarseLaneSuppressesWholeCohortThenRecovers(t *testing.T) {
	idx := buildTraceIndex(t, "block-ambiguous.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
 io-40 (40) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
 io-40 (40) [003] .... 1.002000: block_rq_complete: 8,0 R () 123 + 8 [0]
 io-40 (40) [003] .... 1.003000: block_rq_complete: 8,0 R () 123 + 8 [0]
 io-40 (40) [003] .... 1.004000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
 io-40 (40) [003] .... 1.005000: block_rq_complete: 8,0 R () 123 + 8 [0]
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.006})
	if len(stats.IOLatencies) != 1 || !near(stats.IOLatencies[0].IssueTs, 1.004, 1e-9) || !near(stats.IOLatencies[0].DurationMs, 1, 0.001) {
		t.Fatalf("ambiguous cohort minted latency or prevented recovery: %+v", stats.IOLatencies)
	}
	row := storageLatencyRow(stats.StorageLatencyByLayer, "block", blockEndpointFamilyRQ)
	if row == nil || row.PairedCount != 1 || row.AmbiguousCohortCount != 1 || row.PairingSuppressedCount != 2 {
		t.Fatalf("block ambiguity quality accounting: %+v", stats.StorageLatencyByLayer)
	}
	if !containsSubstring(stats.Caveats, "block_io_pairing_ambiguous=true") {
		t.Fatalf("ambiguous block cohort lacks caveat: %+v", stats.Caveats)
	}
}

func TestBlockAmbiguousCohortTruncatedAtEOFCountsEachStartOnce(t *testing.T) {
	idx := buildTraceIndex(t, "block-ambiguous-eof.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
 io-40 (40) [003] .... 1.001000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
 io-40 (40) [003] .... 1.002000: block_rq_complete: 8,0 R () 123 + 8 [0]
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.003})
	row := storageLatencyRow(stats.StorageLatencyByLayer, "block", blockEndpointFamilyRQ)
	if len(stats.IOLatencies) != 0 || row == nil || row.AmbiguousCohortCount != 1 || row.PairingSuppressedCount != 2 {
		t.Fatalf("EOF-truncated cohort must suppress its two starts exactly once: latencies=%+v rows=%+v", stats.IOLatencies, stats.StorageLatencyByLayer)
	}
}

func TestBlockPairingNeverCrossesPhysicalArtifacts(t *testing.T) {
	fields := func() *BlockIOFields {
		return &BlockIOFields{Dev: "8,0", Op: "R", Sector: 123, Len: 8, IdentityParsed: true, IdentityValid: true}
	}
	idx := &Index{
		TimestampOrder: TraceTimestampOrderMonotonic,
		TraceArtifacts: []TraceArtifactSource{
			{SourcePath: "/trace/source-a.systrace", LocalLineCount: 1, VirtualLineBase: 0, CausalCompatible: true},
			{SourcePath: "/trace/source-b.systrace", LocalLineCount: 1, VirtualLineBase: 1, CausalCompatible: true},
		},
		Events: []Event{
			{Line: 1, Ts: 1, Type: EventBlockIssue, Name: "block_rq_issue", PID: 40, BlockIOFields: fields()},
			{Line: 2, Ts: 1.001, Type: EventBlockComplete, Name: "block_rq_complete", PID: 41, BlockIOFields: fields()},
		},
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.002})
	if len(stats.IOLatencies) != 0 {
		t.Fatalf("lookalike endpoints from distinct artifacts were joined: %+v", stats.IOLatencies)
	}
	if !containsSubstring(stats.Caveats, "block_io_pairing_unpaired=true") {
		t.Fatalf("cross-artifact split was not disclosed: %+v", stats.Caveats)
	}
}

func TestStorageConcurrentCoarseLaneSuppressesWholeCohortThenRecovers(t *testing.T) {
	idx := buildTraceIndex(t, "storage-ambiguous.systrace", `
 io-40 (40) [003] .... 1.000000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 1.001000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 1.002000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 1.003000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 1.004000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 1.006000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.007})
	row := storageLatencyRow(stats.StorageLatencyByLayer, "scsi", "scsi_dispatch_cmd")
	if row == nil || row.PairedCount != 1 || !near(row.MaxLatencyMs, 2, 0.001) || row.AmbiguousCohortCount != 1 || row.PairingSuppressedCount != 2 {
		t.Fatalf("storage ambiguity suppression/recovery: %+v", stats.StorageLatencyByLayer)
	}
	if !containsSubstring(stats.Caveats, "storage_latency_pairing_ambiguous=true") {
		t.Fatalf("ambiguous storage cohort lacks caveat: %+v", stats.Caveats)
	}
}

func TestStoragePairingNeverCrossesPhysicalArtifacts(t *testing.T) {
	resource := func() *ResourceFields { return &ResourceFields{Op: "read", Bytes: 4096} }
	file := func() *FileFields { return &FileFields{Dev: "12,80", RW: "read", Len: 4096} }
	idx := &Index{
		TimestampOrder: TraceTimestampOrderMonotonic,
		TraceArtifacts: []TraceArtifactSource{
			{SourcePath: "/trace/storage-a.systrace", LocalLineCount: 1, VirtualLineBase: 0, CausalCompatible: true},
			{SourcePath: "/trace/storage-b.systrace", LocalLineCount: 1, VirtualLineBase: 1, CausalCompatible: true},
		},
		Events: []Event{
			{Line: 1, Ts: 1, Type: EventStorage, Name: "scsi_dispatch_cmd_start", PID: 40, ResourceFields: resource(), FileFields: file()},
			{Line: 2, Ts: 1.001, Type: EventStorage, Name: "scsi_dispatch_cmd_done", PID: 40, ResourceFields: resource(), FileFields: file()},
		},
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.002})
	paired, starts, dones := 0, 0, 0
	for _, row := range stats.StorageLatencyByLayer {
		if row.Layer != "scsi" {
			continue
		}
		paired += row.PairedCount
		starts += row.UnpairedStartCount
		dones += row.UnpairedDoneCount
	}
	if paired != 0 || starts != 1 || dones != 1 {
		t.Fatalf("lookalike storage endpoints crossed artifacts: %+v", stats.StorageLatencyByLayer)
	}
	if !containsSubstring(stats.Caveats, "storage_latency_pairing_unpaired=true") {
		t.Fatalf("cross-artifact storage split was not disclosed: %+v", stats.Caveats)
	}
}

func TestBlockAndStorageSinglePairsDoNotRegress(t *testing.T) {
	idx := buildTraceIndex(t, "io-valid-pairs.systrace", `
 io-40 (40) [003] .... 1.000000: block_bio_queue: 8,0 R 123 + 8 [io]
 io-40 (40) [003] .... 1.001000: block_bio_complete: 8,0 R 123 + 8 [0]
 io-40 (40) [003] .... 1.002000: f2fs_direct_IO_enter: dev=260:136 ino=0x1 pos=0 len=4096 rw=read
 io-40 (40) [003] .... 1.004000: f2fs_direct_IO_exit: dev=260:136 ino=0x1 pos=0 len=4096 rw=read ret=4096
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.005})
	if len(stats.IOLatencies) != 1 || !near(stats.IOLatencies[0].DurationMs, 1, 0.001) ||
		stats.IOLatencies[0].EndpointFamily != blockEndpointFamilyBIO || stats.IOLatencies[0].WaitCaliber != BlockIOWaitCaliberBIOQueueToComplete {
		t.Fatalf("valid bio pair regressed: %+v", stats.IOLatencies)
	}
	block := storageLatencyRow(stats.StorageLatencyByLayer, "block", blockEndpointFamilyBIO)
	f2fs := storageLatencyRow(stats.StorageLatencyByLayer, "f2fs", "f2fs_direct_io")
	if block == nil || block.PairedCount != 1 || f2fs == nil || f2fs.PairedCount != 1 || !near(f2fs.MaxLatencyMs, 2, 0.001) {
		t.Fatalf("valid block/storage summaries regressed: %+v", stats.StorageLatencyByLayer)
	}
}

func TestMalformedBlockIdentityFailsClosedWithoutCollapsingToSectorZero(t *testing.T) {
	idx := buildTraceIndex(t, "block-invalid.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 999999999999999999999999 + 8 [io]
 io-40 (40) [003] .... 1.001000: block_rq_complete: 8,0 R () 0 + 8 [0]
 io-40 (40) [003] .... 1.002000: block_rq_issue: 8,0 R 4096 () 1 + 0 [io]
`)
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.003})
	if len(stats.IOLatencies) != 0 {
		t.Fatalf("overflow/missing identity collapsed into a valid pair: %+v", stats.IOLatencies)
	}
	if !containsSubstring(stats.Caveats, "block_io_pairing_identity_invalid=true") {
		t.Fatalf("invalid production identity lacks fail-closed caveat: %+v", stats.Caveats)
	}
}

func TestBlockPairingReplaysClosedPrefixWithoutFalseCarryIn(t *testing.T) {
	idx := buildTraceIndex(t, "block-prefix-replay.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 1.001000: block_rq_complete: 8,0 R () 123 + 8 [0]
 io-40 (40) [003] .... 2.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 2.002000: block_rq_complete: 8,0 R () 123 + 8 [0]
`)
	q := Query{TimeStart: 1.5, TimeEnd: 2.1, TimeStartSet: true, TimeEndSet: true}
	stats := ComputeWindowStats(idx, q)
	if len(stats.IOLatencies) != 1 || !near(stats.IOLatencies[0].IssueTs, 2, 1e-9) || !near(stats.IOLatencies[0].DurationMs, 2, 0.001) {
		t.Fatalf("closed pre-window pair polluted the in-window lane: %+v", stats.IOLatencies)
	}
	row := storageLatencyRow(stats.StorageLatencyByLayer, "block", blockEndpointFamilyRQ)
	if row == nil || row.PairedCount != 1 || row.AmbiguousCohortCount != 0 || row.UnpairedStartCount != 0 {
		t.Fatalf("closed prefix became false carry-in: %+v caveats=%+v", row, stats.Caveats)
	}
}

func TestBlockPairingClosedPrefixAloneProducesNoWindowRow(t *testing.T) {
	idx := buildTraceIndex(t, "block-prefix-only.systrace", `
 io-40 (40) [003] .... 1.000000: block_rq_issue: 8,0 R 4096 () 123 + 8 [io]
irq-2 (2) [003] .... 1.001000: block_rq_complete: 8,0 R () 123 + 8 [0]
`)
	q := Query{TimeStart: 1.5, TimeEnd: 2.0, TimeStartSet: true, TimeEndSet: true}
	stats := ComputeWindowStats(idx, q)
	if len(stats.IOLatencies) != 0 || storageLatencyRow(stats.StorageLatencyByLayer, "block", blockEndpointFamilyRQ) != nil {
		t.Fatalf("fully closed prefix leaked into the selected window: latencies=%+v storage=%+v", stats.IOLatencies, stats.StorageLatencyByLayer)
	}
	if containsSubstring(stats.Caveats, "block_io_pairing_unpaired=true") || containsSubstring(stats.Caveats, "block_io_pairing_ambiguous=true") {
		t.Fatalf("fully closed prefix minted a window caveat: %+v", stats.Caveats)
	}
}

func TestGenericStoragePairingCarriesExactOpenPrefixIntoWindow(t *testing.T) {
	idx := buildTraceIndex(t, "storage-carry-in.systrace", `
 io-40 (40) [003] .... 1.000000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 2.000000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096
`)
	q := Query{TimeStart: 1.5, TimeEnd: 2.1, TimeStartSet: true, TimeEndSet: true}
	stats := ComputeWindowStats(idx, q)
	row := storageLatencyRow(stats.StorageLatencyByLayer, "scsi", "scsi_dispatch_cmd")
	if row == nil || row.PairedCount != 1 || !near(row.MaxLatencyMs, 1000, 0.001) || row.UnpairedDoneCount != 0 {
		t.Fatalf("generic storage carry-in was not reconstructed from the exact prefix: %+v caveats=%+v", row, stats.Caveats)
	}
}

func TestGenericStorageClosedPrefixDoesNotPolluteWindow(t *testing.T) {
	idx := buildTraceIndex(t, "storage-prefix-replay.systrace", `
 io-40 (40) [003] .... 1.000000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 1.001000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 2.000000: scsi_dispatch_cmd_start: dev=12,80 op=read bytes=4096
 io-40 (40) [003] .... 2.003000: scsi_dispatch_cmd_done: dev=12,80 op=read bytes=4096
`)
	q := Query{TimeStart: 1.5, TimeEnd: 2.1, TimeStartSet: true, TimeEndSet: true}
	stats := ComputeWindowStats(idx, q)
	row := storageLatencyRow(stats.StorageLatencyByLayer, "scsi", "scsi_dispatch_cmd")
	if row == nil || row.PairedCount != 1 || !near(row.MaxLatencyMs, 3, 0.001) || row.Count != 2 || row.AmbiguousCohortCount != 0 {
		t.Fatalf("generic storage closed prefix polluted window accounting: %+v", row)
	}
}

func TestBlockTypedTupleKeyCannotCollideAndInvalidTokensFailClosed(t *testing.T) {
	if (blockRequestIdentity{Family: "block", Dev: "a/b", Op: "c", Sector: 1, Len: 1}).laneKey() ==
		(blockRequestIdentity{Family: "block", Dev: "a", Op: "b/c", Sector: 1, Len: 1}).laneKey() {
		t.Fatal("typed tuple dimensions collapsed through display punctuation")
	}
	fields := func(dev, op string) *BlockIOFields {
		return &BlockIOFields{Dev: dev, Op: op, Sector: 1, Len: 1, IdentityParsed: true, IdentityValid: true}
	}
	idx := &Index{Path: "/trace/key-collision.systrace", TimestampOrder: TraceTimestampOrderMonotonic, LastTs: 1.001, LineCount: 2, Events: []Event{
		{Line: 1, Ts: 1, Type: EventBlockIssue, Name: "block_rq_issue", PID: 40, BlockIOFields: fields("a/b", "c")},
		{Line: 2, Ts: 1.001, Type: EventBlockComplete, Name: "block_rq_complete", PID: 2, BlockIOFields: fields("a", "b/c")},
	}}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true})
	if len(stats.IOLatencies) != 0 {
		t.Fatalf("slash-bearing tuple dimensions collided into a false pair: %+v", stats.IOLatencies)
	}
	if !containsSubstring(stats.Caveats, "block_io_pairing_identity_invalid=true") {
		t.Fatalf("slash-bearing non-wire identities were not rejected explicitly: %+v", stats.Caveats)
	}
}

func storageLatencyRow(rows []StorageLatencySummary, layer, event string) *StorageLatencySummary {
	for i := range rows {
		if rows[i].Layer == layer && rows[i].Event == event {
			return &rows[i]
		}
	}
	return nil
}
