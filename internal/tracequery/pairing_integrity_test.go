package tracequery

import (
	"fmt"
	"sort"
	"testing"
)

func integrityWorkqueueEvent(line int, ts float64, pid int, name string, work int) Event {
	return Event{Line: line, Ts: ts, PID: pid, Type: EventWorkqueue, Name: name, FieldText: fmt.Sprintf("work=0x%08x", work)}
}

func integrityDMAEvent(line int, ts float64, pid int, name string, context, seqno int) Event {
	return Event{Line: line, Ts: ts, PID: pid, Type: EventDMAFence, Name: name, FieldText: fmt.Sprintf("driver=gpu timeline=t context=%d seqno=%d", context, seqno)}
}

func integrityBlockEvent(line int, ts float64, pid int, name string, sector int64) Event {
	field := fmt.Sprintf("8,0 R 512 () %d + 1 [io]", sector)
	typ := EventBlockIssue
	if name == "block_rq_complete" {
		typ = EventBlockComplete
		field = fmt.Sprintf("8,0 R () %d + 1 [0]", sector)
	}
	return Event{
		Line: line, Ts: ts, PID: pid, Type: typ, Name: name, FieldText: field,
		BlockIOFields: &BlockIOFields{Dev: "8,0", Op: "R", Sector: sector, Len: 1, IdentityParsed: true, IdentityValid: true},
	}
}

func integrityStorageEvent(line int, ts float64, pid int, name, dev string) Event {
	return Event{
		Line: line, Ts: ts, PID: pid, Type: EventStorage, Name: name,
		FieldText:      fmt.Sprintf("dev=%s tag=1 lba=2 len=4096 opcode=READ_10", dev),
		ResourceFields: &ResourceFields{Op: "read", Bytes: 4096},
		FileFields:     &FileFields{Dev: dev, RW: "read", Len: 4096},
	}
}

func integrityDirectCompositeIndexes(events []Event) []struct {
	name string
	idx  *Index
} {
	directEvents := append([]Event(nil), events...)
	compositeEvents := append([]Event(nil), events...)
	sort.SliceStable(compositeEvents, func(i, j int) bool {
		if compositeEvents[i].Ts != compositeEvents[j].Ts {
			return compositeEvents[i].Ts < compositeEvents[j].Ts
		}
		return compositeEvents[i].Line < compositeEvents[j].Line
	})
	lineCount := 0
	for _, ev := range events {
		if ev.Line > lineCount {
			lineCount = ev.Line
		}
	}
	return []struct {
		name string
		idx  *Index
	}{
		{
			name: "direct",
			idx: &Index{
				Path: "/trace/physical-child.systrace", TimestampOrder: TraceTimestampOrderMonotonic,
				LineCount: lineCount, Events: directEvents,
			},
		},
		{
			name: "composite",
			idx: &Index{
				Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic,
				LineCount: lineCount,
				TraceArtifacts: []TraceArtifactSource{{
					SourcePath: "/trace/physical-child.systrace", LocalLineCount: lineCount, VirtualLineBase: 0, CausalCompatible: true,
				}},
				Events: compositeEvents,
			},
		},
	}
}

func TestPairingSequentialReuseRollbackQuarantinesOnlyExactLane(t *testing.T) {
	t.Parallel()
	idx := &Index{Path: "/trace/rollback.systrace", TimestampOrder: TraceTimestampOrderRegressed, Events: []Event{
		integrityWorkqueueEvent(1, 2.0, 10, "workqueue_execute_start", 1),
		integrityWorkqueueEvent(2, 3.0, 10, "workqueue_execute_end", 1),
		integrityWorkqueueEvent(3, 1.0, 10, "workqueue_execute_start", 1),
		integrityWorkqueueEvent(4, 1.5, 10, "workqueue_execute_end", 1),
		integrityWorkqueueEvent(5, 4.0, 11, "workqueue_execute_start", 2),
		integrityWorkqueueEvent(6, 5.0, 11, "workqueue_execute_end", 2),
		integrityDMAEvent(7, 2.0, 20, "dma_fence_wait_start", 1, 1),
		integrityDMAEvent(8, 3.0, 20, "dma_fence_wait_end", 1, 1),
		integrityDMAEvent(9, 1.0, 20, "dma_fence_wait_start", 1, 1),
		integrityDMAEvent(10, 1.5, 20, "dma_fence_wait_end", 1, 1),
		integrityDMAEvent(11, 4.0, 21, "dma_fence_wait_start", 2, 2),
		integrityDMAEvent(12, 5.0, 21, "dma_fence_wait_end", 2, 2),
		integrityBlockEvent(13, 2.0, 30, "block_rq_issue", 10),
		integrityBlockEvent(14, 3.0, 30, "block_rq_complete", 10),
		integrityBlockEvent(15, 1.0, 30, "block_rq_issue", 10),
		integrityBlockEvent(16, 1.5, 30, "block_rq_complete", 10),
		integrityBlockEvent(17, 4.0, 31, "block_rq_issue", 20),
		integrityBlockEvent(18, 5.0, 31, "block_rq_complete", 20),
		integrityStorageEvent(19, 2.0, 40, "scsi_dispatch_cmd_start", "12,80"),
		integrityStorageEvent(20, 3.0, 40, "scsi_dispatch_cmd_done", "12,80"),
		integrityStorageEvent(21, 1.0, 40, "scsi_dispatch_cmd_start", "12,80"),
		integrityStorageEvent(22, 1.5, 40, "scsi_dispatch_cmd_done", "12,80"),
		integrityStorageEvent(23, 4.0, 41, "scsi_dispatch_cmd_start", "12,81"),
		integrityStorageEvent(24, 5.0, 41, "scsi_dispatch_cmd_done", "12,81"),
	}}
	stats := ComputeWindowStats(idx, Query{TimeStart: .5, TimeEnd: 5.5})
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].Thread.PID != 11 || stats.WorkqueueActivity[0].PairedCount != 1 {
		t.Fatalf("workqueue rollback lane was rescued or sibling lost: %+v", stats.WorkqueueActivity)
	}
	if len(stats.DMAFenceActivity) != 1 || stats.DMAFenceActivity[0].Thread.PID != 21 || stats.DMAFenceActivity[0].PairedCount != 1 {
		t.Fatalf("DMA rollback lane was rescued or sibling lost: %+v", stats.DMAFenceActivity)
	}
	if len(stats.IOLatencies) != 1 || stats.IOLatencies[0].Sector != 20 {
		t.Fatalf("block rollback lane was rescued or sibling lost: %+v", stats.IOLatencies)
	}
	var scsi *StorageLatencySummary
	for i := range stats.StorageLatencyByLayer {
		if stats.StorageLatencyByLayer[i].Layer == "scsi" {
			scsi = &stats.StorageLatencyByLayer[i]
		}
	}
	if scsi == nil || scsi.Thread.PID != 41 || scsi.PairedCount != 1 {
		t.Fatalf("storage rollback lane was rescued or sibling lost: %+v", stats.StorageLatencyByLayer)
	}
	if !containsSubstring(stats.Caveats, "duration_pairing_exact_lane_quarantined=true") {
		t.Fatalf("exact rollback quarantine was not disclosed: %v", stats.Caveats)
	}
}

func TestPairingUnknownKeyPoisonsOnlyPhysicalSourceFamily(t *testing.T) {
	t.Parallel()
	badBlock := Event{Line: 3, Ts: 1.0, PID: -1, Type: EventBlockIssue, Name: "block_rq_issue", FieldText: "malformed", BlockIOFields: &BlockIOFields{}}
	badStorage := integrityStorageEvent(4, 1.0, -1, "scsi_dispatch_cmd_start", "12,80")
	idx := &Index{
		Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic,
		TraceArtifacts: []TraceArtifactSource{
			{SourcePath: "/trace/a.systrace", LocalLineCount: 10, VirtualLineBase: 0, CausalCompatible: true},
			{SourcePath: "/trace/b.systrace", LocalLineCount: 20, VirtualLineBase: 100, CausalCompatible: true},
		},
		Events: []Event{
			{Line: 1, Ts: 1.0, PID: 10, Type: EventWorkqueue, Name: "workqueue_execute_start", FieldText: "function=missing_work"},
			{Line: 2, Ts: 1.0, PID: 20, Type: EventDMAFence, Name: "dma_fence_wait_start", FieldText: "driver=gpu timeline=t seqno=1"},
			badBlock, badStorage,
			integrityWorkqueueEvent(101, 1.1, 11, "workqueue_execute_start", 2), integrityWorkqueueEvent(102, 1.2, 11, "workqueue_execute_end", 2),
			integrityDMAEvent(103, 1.1, 21, "dma_fence_wait_start", 2, 2), integrityDMAEvent(104, 1.2, 21, "dma_fence_wait_end", 2, 2),
			integrityBlockEvent(105, 1.1, 31, "block_rq_issue", 20), integrityBlockEvent(106, 1.2, 31, "block_rq_complete", 20),
			integrityStorageEvent(107, 1.1, 41, "scsi_dispatch_cmd_start", "12,81"), integrityStorageEvent(108, 1.2, 41, "scsi_dispatch_cmd_done", "12,81"),
		},
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: .9, TimeEnd: 1.3})
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].Thread.PID != 11 || len(stats.DMAFenceActivity) != 1 || stats.DMAFenceActivity[0].Thread.PID != 21 {
		t.Fatalf("source-A poison leaked to source B: work=%+v dma=%+v", stats.WorkqueueActivity, stats.DMAFenceActivity)
	}
	if len(stats.IOLatencies) != 1 || stats.IOLatencies[0].SourcePath != "/trace/b.systrace" {
		t.Fatalf("block source poison leaked or invalid source survived: %+v", stats.IOLatencies)
	}
	var scsiB bool
	for _, row := range stats.StorageLatencyByLayer {
		if row.Layer == "scsi" && row.SourcePath == "/trace/b.systrace" && row.PairedCount == 1 {
			scsiB = true
		}
		if row.Layer == "scsi" && row.SourcePath == "/trace/a.systrace" {
			t.Fatalf("poisoned source-A storage row survived: %+v", row)
		}
	}
	if !scsiB || !containsSubstring(stats.Caveats, "duration_pairing_source_fail_closed=true") {
		t.Fatalf("source-local storage outcome/caveat missing: rows=%+v caveats=%v", stats.StorageLatencyByLayer, stats.Caveats)
	}
}

func TestPairingExactLaneQuarantineIsNamespacedByPhysicalSource(t *testing.T) {
	t.Parallel()
	idx := &Index{
		Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderRegressed,
		TraceArtifacts: []TraceArtifactSource{
			{SourcePath: "/trace/a.systrace", LocalLineCount: 50, VirtualLineBase: 0, CausalCompatible: true},
			{SourcePath: "/trace/b.systrace", LocalLineCount: 50, VirtualLineBase: 100, CausalCompatible: true},
		},
		Events: []Event{
			integrityWorkqueueEvent(1, 2, 10, "workqueue_execute_start", 1), integrityWorkqueueEvent(2, 3, 10, "workqueue_execute_end", 1),
			integrityWorkqueueEvent(3, 1, 10, "workqueue_execute_start", 1), integrityWorkqueueEvent(4, 1.5, 10, "workqueue_execute_end", 1),
			integrityDMAEvent(5, 2, 20, "dma_fence_wait_start", 1, 1), integrityDMAEvent(6, 3, 20, "dma_fence_wait_end", 1, 1),
			integrityDMAEvent(7, 1, 20, "dma_fence_wait_start", 1, 1), integrityDMAEvent(8, 1.5, 20, "dma_fence_wait_end", 1, 1),
			integrityBlockEvent(9, 2, 30, "block_rq_issue", 10), integrityBlockEvent(10, 3, 30, "block_rq_complete", 10),
			integrityBlockEvent(11, 1, 30, "block_rq_issue", 10), integrityBlockEvent(12, 1.5, 30, "block_rq_complete", 10),
			integrityStorageEvent(13, 2, 40, "scsi_dispatch_cmd_start", "12,80"), integrityStorageEvent(14, 3, 40, "scsi_dispatch_cmd_done", "12,80"),
			integrityStorageEvent(15, 1, 40, "scsi_dispatch_cmd_start", "12,80"), integrityStorageEvent(16, 1.5, 40, "scsi_dispatch_cmd_done", "12,80"),
			integrityWorkqueueEvent(101, 4, 10, "workqueue_execute_start", 1), integrityWorkqueueEvent(102, 5, 10, "workqueue_execute_end", 1),
			integrityDMAEvent(103, 4, 20, "dma_fence_wait_start", 1, 1), integrityDMAEvent(104, 5, 20, "dma_fence_wait_end", 1, 1),
			integrityBlockEvent(105, 4, 30, "block_rq_issue", 10), integrityBlockEvent(106, 5, 30, "block_rq_complete", 10),
			integrityStorageEvent(107, 4, 40, "scsi_dispatch_cmd_start", "12,80"), integrityStorageEvent(108, 5, 40, "scsi_dispatch_cmd_done", "12,80"),
		},
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: .5, TimeEnd: 5.5})
	if len(stats.WorkqueueActivity) != 1 || stats.WorkqueueActivity[0].SourcePath != "/trace/b.systrace" ||
		len(stats.DMAFenceActivity) != 1 || stats.DMAFenceActivity[0].SourcePath != "/trace/b.systrace" ||
		len(stats.IOLatencies) != 1 || stats.IOLatencies[0].SourcePath != "/trace/b.systrace" {
		t.Fatalf("same semantic key crossed source quarantine: work=%+v dma=%+v block=%+v", stats.WorkqueueActivity, stats.DMAFenceActivity, stats.IOLatencies)
	}
	var storageB bool
	for _, row := range stats.StorageLatencyByLayer {
		if row.Layer == "scsi" && row.SourcePath == "/trace/b.systrace" && row.PairedCount == 1 {
			storageB = true
		}
		if row.Layer == "scsi" && row.SourcePath == "/trace/a.systrace" {
			t.Fatalf("source-A exact storage lane survived: %+v", row)
		}
	}
	if !storageB {
		t.Fatalf("source-B same-key storage pair was lost: %+v", stats.StorageLatencyByLayer)
	}
}

func TestPairingReplayAuditBlocksHeadAndTailRescue(t *testing.T) {
	t.Parallel()
	badBlock := Event{Line: 2, Ts: 3, PID: 30, Type: EventBlockComplete, Name: "block_rq_complete", FieldText: "malformed", BlockIOFields: &BlockIOFields{}}
	blockIdx := &Index{Path: "/trace/block-carry.systrace", TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
		integrityBlockEvent(1, .5, 30, "block_rq_issue", 10), badBlock, integrityBlockEvent(3, 4, 30, "block_rq_complete", 10),
	}}
	block := ComputeWindowStats(blockIdx, Query{TimeStart: 1, TimeEnd: 2})
	if len(block.IOLatencies) != 0 || !containsSubstring(block.Caveats, "duration_pairing_source_fail_closed=true") {
		t.Fatalf("block carry interval bridged malformed suffix endpoint: rows=%+v caveats=%v", block.IOLatencies, block.Caveats)
	}
	badStorage := integrityStorageEvent(2, 3, -1, "scsi_dispatch_cmd_done", "12,80")
	storageIdx := &Index{Path: "/trace/storage-carry.systrace", TimestampOrder: TraceTimestampOrderMonotonic, Events: []Event{
		integrityStorageEvent(1, .5, 40, "scsi_dispatch_cmd_start", "12,80"), badStorage, integrityStorageEvent(3, 4, 40, "scsi_dispatch_cmd_done", "12,80"),
	}}
	storage := ComputeWindowStats(storageIdx, Query{TimeStart: 1, TimeEnd: 2})
	for _, row := range storage.StorageLatencyByLayer {
		if row.Layer == "scsi" {
			t.Fatalf("storage carry interval bridged malformed suffix endpoint: %+v", storage.StorageLatencyByLayer)
		}
	}
	if !containsSubstring(storage.Caveats, "duration_pairing_source_fail_closed=true") {
		t.Fatalf("storage carry anti-rescue was not disclosed: %v", storage.Caveats)
	}
}

func TestBlockStorageLineScopeReplaysHeadAndTailOverlapAcrossEntryPoints(t *testing.T) {
	t.Parallel()
	q := Query{LineStart: 2, LineEnd: 3}
	blockEvents := []Event{
		integrityBlockEvent(1, 1.0, 30, "block_rq_issue", 10),
		integrityBlockEvent(2, 1.1, 30, "block_rq_issue", 10),
		integrityBlockEvent(3, 1.2, 30, "block_rq_complete", 10),
		integrityBlockEvent(4, 1.3, 30, "block_rq_complete", 10),
	}
	for _, tc := range integrityDirectCompositeIndexes(blockEvents) {
		tc := tc
		t.Run("block/"+tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeBlockIOLatencies(tc.idx, q, 8)
			if len(got.latencies) != 0 || !containsSubstring(got.caveats, "block_io_pairing_ambiguous=true") {
				t.Fatalf("line-scope cropped the physical block cohort into a false pair: latencies=%+v summaries=%+v caveats=%v", got.latencies, got.summaries, got.caveats)
			}
		})
	}

	storageEvents := []Event{
		integrityStorageEvent(1, 1.0, 40, "scsi_dispatch_cmd_start", "12,80"),
		integrityStorageEvent(2, 1.1, 40, "scsi_dispatch_cmd_start", "12,80"),
		integrityStorageEvent(3, 1.2, 40, "scsi_dispatch_cmd_done", "12,80"),
		integrityStorageEvent(4, 1.3, 40, "scsi_dispatch_cmd_done", "12,80"),
	}
	for _, tc := range integrityDirectCompositeIndexes(storageEvents) {
		tc := tc
		t.Run("storage/"+tc.name, func(t *testing.T) {
			t.Parallel()
			rows, caveats := computeStorageLatencyByLayer(tc.idx, q, nil, 8)
			if row := storageLatencyRow(rows, "scsi", "scsi_dispatch_cmd"); row == nil || row.PairedCount != 0 || row.PairingSuppressedCount != 2 ||
				!containsSubstring(caveats, "storage_latency_pairing_ambiguous=true") {
				t.Fatalf("line-scope cropped the physical storage cohort into a false pair: rows=%+v caveats=%v", rows, caveats)
			}
		})
	}
}

func TestBlockStorageLineScopeCannotDeleteOutsideMalformedEndpoint(t *testing.T) {
	t.Parallel()
	q := Query{LineStart: 2, LineEnd: 3}
	badBlock := integrityBlockEvent(1, 1.0, 30, "block_rq_issue", 10)
	// Bytes are non-key payload. Overflow keeps the exact dev/op/sector/len
	// lane known while rejecting admission, so deletion would rescue line 2-3.
	badBlock.FieldText = "8,0 R 4294967296 () 10 + 1 [io]"
	blockEvents := []Event{
		badBlock,
		integrityBlockEvent(2, 1.1, 30, "block_rq_issue", 10),
		integrityBlockEvent(3, 1.2, 30, "block_rq_complete", 10),
	}
	for _, tc := range integrityDirectCompositeIndexes(blockEvents) {
		tc := tc
		t.Run("block/"+tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeBlockIOLatencies(tc.idx, q, 8)
			if len(got.latencies) != 0 || !containsSubstring(got.caveats, "duration_pairing_exact_lane_quarantined=true") {
				t.Fatalf("outside malformed block endpoint was deleted then bridged: latencies=%+v caveats=%v", got.latencies, got.caveats)
			}
		})
	}

	badStorage := integrityStorageEvent(1, 1.0, 40, "scsi_dispatch_cmd_start", "12,80")
	// SCSI tag is payload rather than coarse identity. A malformed tag must
	// quarantine the known dev/base/PID lane even when the row is outside q.
	badStorage.FieldText = "dev=12,80 tag=bad lba=2 len=4096 opcode=READ_10"
	storageEvents := []Event{
		badStorage,
		integrityStorageEvent(2, 1.1, 40, "scsi_dispatch_cmd_start", "12,80"),
		integrityStorageEvent(3, 1.2, 40, "scsi_dispatch_cmd_done", "12,80"),
	}
	for _, tc := range integrityDirectCompositeIndexes(storageEvents) {
		tc := tc
		t.Run("storage/"+tc.name, func(t *testing.T) {
			t.Parallel()
			rows, caveats := computeStorageLatencyByLayer(tc.idx, q, nil, 8)
			if row := storageLatencyRow(rows, "scsi", "scsi_dispatch_cmd"); row != nil ||
				!containsSubstring(caveats, "duration_pairing_exact_lane_quarantined=true") {
				t.Fatalf("outside malformed storage endpoint was deleted then bridged: rows=%+v caveats=%v", rows, caveats)
			}
		})
	}
}

func TestHardPairingRawLedgerBarrierCannotBeScopedOut(t *testing.T) {
	t.Parallel()
	type familyCase struct {
		name   string
		family durationOrderFamily
		events []Event
		assert func(t *testing.T, idx *Index, q Query)
	}
	cases := []familyCase{
		{
			name: "workqueue", family: durationOrderWorkqueue,
			events: []Event{
				integrityWorkqueueEvent(2, 2.1, 10, "workqueue_execute_start", 1),
				integrityWorkqueueEvent(3, 2.2, 10, "workqueue_execute_end", 1),
			},
			assert: func(t *testing.T, idx *Index, q Query) {
				rows, caveats := computeWorkqueueActivity(idx, q, 8)
				if len(rows) != 0 || !containsSubstring(caveats, "duration_pairing_exact_lane_quarantined=true") {
					t.Fatalf("raw workqueue barrier was scoped out: rows=%+v caveats=%v", rows, caveats)
				}
			},
		},
		{
			name: "dma_fence", family: durationOrderDMAFence,
			events: []Event{
				integrityDMAEvent(2, 2.1, 20, "dma_fence_wait_start", 1, 1),
				integrityDMAEvent(3, 2.2, 20, "dma_fence_wait_end", 1, 1),
			},
			assert: func(t *testing.T, idx *Index, q Query) {
				rows, caveats := computeDMAFenceActivity(idx, q, 8)
				if len(rows) != 0 || !containsSubstring(caveats, "duration_pairing_exact_lane_quarantined=true") {
					t.Fatalf("raw DMA barrier was scoped out: rows=%+v caveats=%v", rows, caveats)
				}
			},
		},
		{
			name: "block", family: durationOrderBlockIO,
			events: []Event{
				integrityBlockEvent(2, 2.1, 30, "block_rq_issue", 10),
				integrityBlockEvent(3, 2.2, 30, "block_rq_complete", 10),
			},
			assert: func(t *testing.T, idx *Index, q Query) {
				got := computeBlockIOLatencies(idx, q, 8)
				if len(got.latencies) != 0 || !containsSubstring(got.caveats, "duration_pairing_exact_lane_quarantined=true") {
					t.Fatalf("raw block barrier was scoped out: latencies=%+v caveats=%v", got.latencies, got.caveats)
				}
			},
		},
		{
			name: "storage", family: durationOrderStorage,
			events: []Event{
				integrityStorageEvent(2, 2.1, 40, "scsi_dispatch_cmd_start", "12,80"),
				integrityStorageEvent(3, 2.2, 40, "scsi_dispatch_cmd_done", "12,80"),
			},
			assert: func(t *testing.T, idx *Index, q Query) {
				rows, caveats := computeStorageLatencyByLayer(idx, q, nil, 8)
				if row := storageLatencyRow(rows, "scsi", "scsi_dispatch_cmd"); row != nil ||
					!containsSubstring(caveats, "duration_pairing_exact_lane_quarantined=true") {
					t.Fatalf("raw storage barrier was scoped out: rows=%+v caveats=%v", rows, caveats)
				}
			},
		},
	}
	scopes := []struct {
		name string
		q    Query
	}{
		{name: "line", q: Query{LineStart: 2, LineEnd: 3}},
		{name: "time", q: Query{TimeStart: 2, TimeEnd: 3, TimeStartSet: true, TimeEndSet: true}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			verdict := fingerprintPairingEvent(tc.events[0])
			if !verdict.KeyKnown || !verdict.PayloadAdmitted || !verdict.EmitterAdmitted {
				t.Fatalf("control endpoint did not have a hard pairing lane: %+v", verdict)
			}
			const source = "/trace/raw-barrier.systrace"
			idx := &Index{
				Path: source, TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 3,
				Events: append([]Event(nil), tc.events...),
				durationOrderFailures: []durationOrderViolation{{
					Family: tc.family, LaneKey: verdict.SemanticKey, SourcePath: source,
					Issue: "endpoint_parse_incomplete", EventName: tc.events[0].Name,
					Fields: []string{"parser_rejected_row"}, Line: 1, CurrentTs: 1,
				}},
			}
			for _, scope := range scopes {
				scope := scope
				t.Run(scope.name, func(t *testing.T) {
					t.Parallel()
					tc.assert(t, idx, scope.q)
				})
			}
		})
	}
}

func TestBlockStorageFullReplayKeepsLegalLineScopedSequentialReuse(t *testing.T) {
	t.Parallel()
	q := Query{LineStart: 3, LineEnd: 4}
	blockEvents := []Event{
		integrityBlockEvent(1, 1.0, 30, "block_rq_issue", 10), integrityBlockEvent(2, 1.1, 30, "block_rq_complete", 10),
		integrityBlockEvent(3, 2.0, 30, "block_rq_issue", 10), integrityBlockEvent(4, 2.1, 30, "block_rq_complete", 10),
		integrityBlockEvent(5, 3.0, 30, "block_rq_issue", 10), integrityBlockEvent(6, 3.1, 30, "block_rq_complete", 10),
	}
	for _, tc := range integrityDirectCompositeIndexes(blockEvents) {
		tc := tc
		t.Run("block/"+tc.name, func(t *testing.T) {
			t.Parallel()
			got := computeBlockIOLatencies(tc.idx, q, 8)
			if len(got.latencies) != 1 || got.latencies[0].IssueLine != 3 || got.latencies[0].CompleteLine != 4 ||
				containsSubstring(got.caveats, "pairing_ambiguous=true") || containsSubstring(got.caveats, "duration_pairing_exact_lane_quarantined=true") {
				t.Fatalf("complete replay over-suppressed legal block identity reuse: latencies=%+v caveats=%v", got.latencies, got.caveats)
			}
		})
	}

	storageEvents := []Event{
		integrityStorageEvent(1, 1.0, 40, "scsi_dispatch_cmd_start", "12,80"), integrityStorageEvent(2, 1.1, 40, "scsi_dispatch_cmd_done", "12,80"),
		integrityStorageEvent(3, 2.0, 40, "scsi_dispatch_cmd_start", "12,80"), integrityStorageEvent(4, 2.1, 40, "scsi_dispatch_cmd_done", "12,80"),
		integrityStorageEvent(5, 3.0, 40, "scsi_dispatch_cmd_start", "12,80"), integrityStorageEvent(6, 3.1, 40, "scsi_dispatch_cmd_done", "12,80"),
	}
	for _, tc := range integrityDirectCompositeIndexes(storageEvents) {
		tc := tc
		t.Run("storage/"+tc.name, func(t *testing.T) {
			t.Parallel()
			rows, caveats := computeStorageLatencyByLayer(tc.idx, q, nil, 8)
			row := storageLatencyRow(rows, "scsi", "scsi_dispatch_cmd")
			if row == nil || row.PairedCount != 1 || row.LineStart != 3 || row.LineEnd != 4 ||
				containsSubstring(caveats, "pairing_ambiguous=true") || containsSubstring(caveats, "duration_pairing_exact_lane_quarantined=true") {
				t.Fatalf("complete replay over-suppressed legal storage identity reuse: rows=%+v caveats=%v", rows, caveats)
			}
		})
	}
}

func TestWorkqueueDMAFullReplayBlocksWindowExternalOverlap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		events []Event
		assert func(*testing.T, *Index, Query)
	}{
		{
			name: "workqueue",
			events: []Event{
				integrityWorkqueueEvent(1, 1.0, 10, "workqueue_execute_start", 1),
				integrityWorkqueueEvent(2, 2.1, 10, "workqueue_execute_start", 1),
				integrityWorkqueueEvent(3, 2.2, 10, "workqueue_execute_end", 1),
				integrityWorkqueueEvent(4, 3.1, 10, "workqueue_execute_end", 1),
			},
			assert: func(t *testing.T, idx *Index, q Query) {
				rows, caveats := computeWorkqueueActivity(idx, q, 8)
				if len(rows) != 1 || rows[0].PairedCount != 0 || rows[0].DurationMs != 0 || rows[0].AmbiguousCohortCount != 1 ||
					!containsSubstring(caveats, "workqueue_pairing_ambiguous=true") {
					t.Fatalf("window-external workqueue overlap was cropped into a pair: rows=%+v caveats=%v", rows, caveats)
				}
			},
		},
		{
			name: "dma",
			events: []Event{
				integrityDMAEvent(1, 1.0, 20, "dma_fence_wait_start", 1, 1),
				integrityDMAEvent(2, 2.1, 20, "dma_fence_wait_start", 1, 1),
				integrityDMAEvent(3, 2.2, 20, "dma_fence_wait_end", 1, 1),
				integrityDMAEvent(4, 3.1, 20, "dma_fence_wait_end", 1, 1),
			},
			assert: func(t *testing.T, idx *Index, q Query) {
				rows, caveats := computeDMAFenceActivity(idx, q, 8)
				if len(rows) != 1 || rows[0].PairedCount != 0 || rows[0].WaitMs != 0 || rows[0].AmbiguousCohortCount != 1 ||
					!containsSubstring(caveats, "dma_fence_pairing_ambiguous=true") {
					t.Fatalf("window-external DMA overlap was cropped into a pair: rows=%+v caveats=%v", rows, caveats)
				}
			},
		},
	}
	scopes := []struct {
		name string
		q    Query
	}{
		{name: "time", q: Query{TimeStart: 2, TimeEnd: 3, TimeStartSet: true, TimeEndSet: true}},
		{name: "line", q: Query{LineStart: 2, LineEnd: 3}},
	}
	for _, tc := range tests {
		tc := tc
		for _, indexCase := range integrityDirectCompositeIndexes(tc.events) {
			indexCase := indexCase
			for _, scope := range scopes {
				scope := scope
				t.Run(tc.name+"/"+indexCase.name+"/"+scope.name, func(t *testing.T) {
					t.Parallel()
					tc.assert(t, indexCase.idx, scope.q)
				})
			}
		}
	}
}

func TestWorkqueueDMAFullReplayKeepsLegalSequentialReuse(t *testing.T) {
	t.Parallel()
	workqueueEvents := []Event{
		integrityWorkqueueEvent(1, 1.0, 10, "workqueue_execute_start", 1), integrityWorkqueueEvent(2, 1.1, 10, "workqueue_execute_end", 1),
		integrityWorkqueueEvent(3, 2.0, 10, "workqueue_execute_start", 1), integrityWorkqueueEvent(4, 2.1, 10, "workqueue_execute_end", 1),
		integrityWorkqueueEvent(5, 3.0, 10, "workqueue_execute_start", 1), integrityWorkqueueEvent(6, 3.1, 10, "workqueue_execute_end", 1),
	}
	for _, tc := range integrityDirectCompositeIndexes(workqueueEvents) {
		tc := tc
		t.Run("workqueue/"+tc.name, func(t *testing.T) {
			t.Parallel()
			rows, caveats := computeWorkqueueActivity(tc.idx, Query{LineStart: 3, LineEnd: 4}, 8)
			if len(rows) != 1 || rows[0].PairedCount != 1 || !near(rows[0].DurationMs, 100, .001) || rows[0].LineStart != 3 || rows[0].LineEnd != 4 ||
				containsSubstring(caveats, "pairing_ambiguous=true") {
				t.Fatalf("complete replay over-suppressed legal workqueue reuse: rows=%+v caveats=%v", rows, caveats)
			}
		})
	}
	dmaEvents := []Event{
		integrityDMAEvent(1, 1.0, 20, "dma_fence_wait_start", 1, 1), integrityDMAEvent(2, 1.1, 20, "dma_fence_wait_end", 1, 1),
		integrityDMAEvent(3, 2.0, 20, "dma_fence_wait_start", 1, 1), integrityDMAEvent(4, 2.1, 20, "dma_fence_wait_end", 1, 1),
		integrityDMAEvent(5, 3.0, 20, "dma_fence_wait_start", 1, 1), integrityDMAEvent(6, 3.1, 20, "dma_fence_wait_end", 1, 1),
	}
	for _, tc := range integrityDirectCompositeIndexes(dmaEvents) {
		tc := tc
		t.Run("dma/"+tc.name, func(t *testing.T) {
			t.Parallel()
			rows, caveats := computeDMAFenceActivity(tc.idx, Query{LineStart: 3, LineEnd: 4}, 8)
			if len(rows) != 1 || rows[0].PairedCount != 1 || !near(rows[0].WaitMs, 100, .001) || rows[0].LineStart != 3 || rows[0].LineEnd != 4 ||
				containsSubstring(caveats, "pairing_ambiguous=true") {
				t.Fatalf("complete replay over-suppressed legal DMA reuse: rows=%+v caveats=%v", rows, caveats)
			}
		})
	}
}

func TestWorkqueueDMALifecycleResetBreaksPhysicalLane(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		events []Event
		assert func(*testing.T, *Index)
	}{
		{
			name: "workqueue",
			events: []Event{
				integrityWorkqueueEvent(1, 2.0, 10, "workqueue_execute_start", 1),
				{Line: 2, Ts: 1.0, Type: EventSchedSwitch, PrevPID: 10, PrevState: "X"},
				integrityWorkqueueEvent(3, 3.0, 10, "workqueue_execute_end", 1),
			},
			assert: func(t *testing.T, idx *Index) {
				rows, caveats := computeWorkqueueActivity(idx, Query{LineStart: 1, LineEnd: 3}, 8)
				if len(rows) != 1 || rows[0].PairedCount != 0 || rows[0].UnpairedStartCount != 1 || rows[0].UnpairedDoneCount != 1 ||
					!containsSubstring(caveats, "workqueue_pairing_lifecycle_reset=true") {
					t.Fatalf("workqueue endpoints crossed a physical lifecycle reset: rows=%+v caveats=%v", rows, caveats)
				}
			},
		},
		{
			name: "dma",
			events: []Event{
				integrityDMAEvent(1, 2.0, 20, "dma_fence_wait_start", 1, 1),
				{Line: 2, Ts: 1.0, Type: EventSchedSwitch, PrevPID: 20, PrevState: "X"},
				integrityDMAEvent(3, 3.0, 20, "dma_fence_wait_end", 1, 1),
			},
			assert: func(t *testing.T, idx *Index) {
				rows, caveats := computeDMAFenceActivity(idx, Query{LineStart: 1, LineEnd: 3}, 8)
				if len(rows) != 1 || rows[0].PairedCount != 0 || rows[0].UnpairedStartCount != 1 || rows[0].UnpairedDoneCount != 1 ||
					!containsSubstring(caveats, "dma_fence_pairing_lifecycle_reset=true") {
					t.Fatalf("DMA endpoints crossed a physical lifecycle reset: rows=%+v caveats=%v", rows, caveats)
				}
			},
		},
	}
	for _, tc := range tests {
		tc := tc
		for _, indexCase := range integrityDirectCompositeIndexes(tc.events) {
			indexCase := indexCase
			t.Run(tc.name+"/"+indexCase.name, func(t *testing.T) {
				t.Parallel()
				tc.assert(t, indexCase.idx)
			})
		}
	}
}

func TestWorkqueueDMALifecycleResetIsPhysicalSourceScoped(t *testing.T) {
	t.Parallel()
	artifacts := []TraceArtifactSource{
		{SourcePath: "/trace/a.systrace", LocalLineCount: 3, VirtualLineBase: 0, CausalCompatible: true},
		{SourcePath: "/trace/b.systrace", LocalLineCount: 2, VirtualLineBase: 100, CausalCompatible: true},
	}
	wq := &Index{Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 5, TraceArtifacts: artifacts, Events: []Event{
		integrityWorkqueueEvent(1, 1, 10, "workqueue_execute_start", 1),
		integrityWorkqueueEvent(101, 1.05, 10, "workqueue_execute_start", 1),
		{Line: 2, Ts: 1.1, Type: EventSchedSwitch, PrevPID: 10, PrevState: "X"},
		integrityWorkqueueEvent(102, 1.15, 10, "workqueue_execute_end", 1),
		integrityWorkqueueEvent(3, 1.2, 10, "workqueue_execute_end", 1),
	}}
	wqRows, wqCaveats := computeWorkqueueActivity(wq, Query{TimeStart: .9, TimeEnd: 1.3}, 8)
	var pairedB bool
	for _, row := range wqRows {
		if row.SourcePath == "/trace/b.systrace" && row.PairedCount == 1 {
			pairedB = true
		}
		if row.SourcePath == "/trace/a.systrace" && row.PairedCount != 0 {
			t.Fatalf("source-A WQ reset failed to break its own lane: %+v", row)
		}
	}
	if !pairedB || !containsSubstring(wqCaveats, "workqueue_pairing_lifecycle_reset=true") {
		t.Fatalf("source-A WQ reset leaked into source B: rows=%+v caveats=%v", wqRows, wqCaveats)
	}
	dma := &Index{Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 5, TraceArtifacts: artifacts, Events: []Event{
		integrityDMAEvent(1, 1, 20, "dma_fence_wait_start", 1, 1),
		integrityDMAEvent(101, 1.05, 20, "dma_fence_wait_start", 1, 1),
		{Line: 2, Ts: 1.1, Type: EventSchedSwitch, PrevPID: 20, PrevState: "X"},
		integrityDMAEvent(102, 1.15, 20, "dma_fence_wait_end", 1, 1),
		integrityDMAEvent(3, 1.2, 20, "dma_fence_wait_end", 1, 1),
	}}
	dmaRows, dmaCaveats := computeDMAFenceActivity(dma, Query{TimeStart: .9, TimeEnd: 1.3}, 8)
	pairedB = false
	for _, row := range dmaRows {
		if row.SourcePath == "/trace/b.systrace" && row.PairedCount == 1 {
			pairedB = true
		}
		if row.SourcePath == "/trace/a.systrace" && row.PairedCount != 0 {
			t.Fatalf("source-A DMA reset failed to break its own lane: %+v", row)
		}
	}
	if !pairedB || !containsSubstring(dmaCaveats, "dma_fence_pairing_lifecycle_reset=true") {
		t.Fatalf("source-A DMA reset leaked into source B: rows=%+v caveats=%v", dmaRows, dmaCaveats)
	}
}

func TestWorkqueueDMAUnresolvedRelevantLifecycleResetFailsClosed(t *testing.T) {
	t.Parallel()
	indexFor := func(events []Event) *Index {
		return &Index{
			Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 3,
			TraceArtifacts: []TraceArtifactSource{
				{SourcePath: "/trace/child-a.systrace", LocalLineCount: 3, VirtualLineBase: 0, CausalCompatible: true},
				{SourcePath: "/trace/ambiguous-reset.systrace", LocalLineCount: 1, VirtualLineBase: 1, CausalCompatible: true},
			},
			Events: events,
		}
	}
	wqRows, wqCaveats := computeWorkqueueActivity(indexFor([]Event{
		integrityWorkqueueEvent(1, 1, 10, "workqueue_execute_start", 1),
		{Line: 2, Ts: 1.5, Type: EventSchedSwitch, PrevPID: 10, PrevState: "X"},
		integrityWorkqueueEvent(3, 2, 10, "workqueue_execute_end", 1),
	}), Query{LineStart: 1, LineEnd: 3}, 8)
	if len(wqRows) != 0 || !containsSubstring(wqCaveats, "duration_pairing_fail_closed=true") ||
		!containsSubstring(wqCaveats, "workqueue_pairing_lifecycle_reset_provenance_unresolved=true") {
		t.Fatalf("unresolved workqueue reset provenance did not fail closed: rows=%+v caveats=%v", wqRows, wqCaveats)
	}
	dmaRows, dmaCaveats := computeDMAFenceActivity(indexFor([]Event{
		integrityDMAEvent(1, 1, 20, "dma_fence_wait_start", 1, 1),
		{Line: 2, Ts: 1.5, Type: EventSchedSwitch, PrevPID: 20, PrevState: "X"},
		integrityDMAEvent(3, 2, 20, "dma_fence_wait_end", 1, 1),
	}), Query{LineStart: 1, LineEnd: 3}, 8)
	if len(dmaRows) != 0 || !containsSubstring(dmaCaveats, "duration_pairing_fail_closed=true") ||
		!containsSubstring(dmaCaveats, "dma_fence_pairing_lifecycle_reset_provenance_unresolved=true") {
		t.Fatalf("unresolved DMA reset provenance did not fail closed: rows=%+v caveats=%v", dmaRows, dmaCaveats)
	}
}

func TestWorkqueueDMALineBoundsOverrideConflictingTimeBounds(t *testing.T) {
	t.Parallel()
	q := Query{LineStart: 1, LineEnd: 2, TimeStart: 100, TimeEnd: 200, TimeStartSet: true, TimeEndSet: true}
	wq := &Index{Path: "/trace/mixed-window-wq.systrace", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 2, Events: []Event{
		integrityWorkqueueEvent(1, 1, 10, "workqueue_execute_start", 1), integrityWorkqueueEvent(2, 2, 10, "workqueue_execute_end", 1),
	}}
	wqRows, wqCaveats := computeWorkqueueActivity(wq, q, 8)
	if len(wqRows) != 1 || wqRows[0].PairedCount != 1 || !near(wqRows[0].DurationMs, 1000, .001) {
		t.Fatalf("conflicting time bounds overrode authoritative WQ line bounds: rows=%+v caveats=%v", wqRows, wqCaveats)
	}
	dma := &Index{Path: "/trace/mixed-window-dma.systrace", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 2, Events: []Event{
		integrityDMAEvent(1, 1, 20, "dma_fence_wait_start", 1, 1), integrityDMAEvent(2, 2, 20, "dma_fence_wait_end", 1, 1),
	}}
	dmaRows, dmaCaveats := computeDMAFenceActivity(dma, q, 8)
	if len(dmaRows) != 1 || dmaRows[0].PairedCount != 1 || !near(dmaRows[0].WaitMs, 1000, .001) {
		t.Fatalf("conflicting time bounds overrode authoritative DMA line bounds: rows=%+v caveats=%v", dmaRows, dmaCaveats)
	}
}

func TestWorkqueueDMAExplicitZeroTimeEndSelectsNoPositiveEndpoint(t *testing.T) {
	t.Parallel()
	q := Query{TimeEnd: 0, TimeEndSet: true}
	wq := &Index{Path: "/trace/zero-end-wq.systrace", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 3, Events: []Event{
		integrityWorkqueueEvent(1, 1, 10, "workqueue_execute_start", 1), integrityWorkqueueEvent(2, 2, 10, "workqueue_execute_end", 1),
		{Line: 3, Ts: 1.5, PID: 10, Type: EventWorkqueue, Name: "workqueue_queue_work_on", FieldText: "work=0x00000001 function=0x2"},
	}}
	if rows, caveats := computeWorkqueueActivity(wq, q, 8); len(rows) != 0 || containsSubstring(caveats, "pairing_ambiguous=true") {
		t.Fatalf("explicit zero time end admitted positive WQ endpoints: rows=%+v caveats=%v", rows, caveats)
	}
	dma := &Index{Path: "/trace/zero-end-dma.systrace", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 3, Events: []Event{
		integrityDMAEvent(1, 1, 20, "dma_fence_wait_start", 1, 1), integrityDMAEvent(2, 2, 20, "dma_fence_wait_end", 1, 1),
		{Line: 3, Ts: 1.5, PID: 20, Type: EventDMAFence, Name: "dma_fence_signaled", FieldText: "driver=gpu timeline=t context=1 seqno=1"},
	}}
	if rows, caveats := computeDMAFenceActivity(dma, q, 8); len(rows) != 0 || containsSubstring(caveats, "pairing_ambiguous=true") {
		t.Fatalf("explicit zero time end admitted positive DMA endpoints: rows=%+v caveats=%v", rows, caveats)
	}
}

func TestGenericStorageLifecycleReplayUsesPhysicalSourceLineOrder(t *testing.T) {
	t.Parallel()
	start := integrityStorageEvent(1, 2.000, 40, "scsi_dispatch_cmd_start", "12,80")
	reset := Event{Line: 2, Ts: 1.000, Type: EventSchedSwitch, PrevPID: 40, PrevState: "X"}
	done := integrityStorageEvent(3, 3.000, 40, "scsi_dispatch_cmd_done", "12,80")
	tests := []struct {
		name string
		idx  *Index
	}{
		{
			name: "direct physical stream",
			idx: &Index{
				Path: "/trace/direct-regressed.systrace", TimestampOrder: TraceTimestampOrderRegressed,
				Events: []Event{start, reset, done},
			},
		},
		{
			name: "composite canonical stream",
			idx: &Index{
				Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 3,
				TraceArtifacts: []TraceArtifactSource{{SourcePath: "/trace/child.systrace", LocalLineCount: 3, VirtualLineBase: 0, CausalCompatible: true}},
				// Bundle merge canonicalizes by timestamp; virtual Line is the
				// retained child-local physical order start -> reset -> done.
				Events: []Event{reset, start, done},
			},
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rows, caveats := computeStorageLatencyByLayer(tc.idx, Query{TimeStart: 2.5, TimeEnd: 3.5}, nil, 8)
			if row := storageLatencyRow(rows, "scsi", "scsi_dispatch_cmd"); row != nil && row.PairedCount > 0 {
				t.Fatalf("storage endpoints crossed a physical lifecycle reset: row=%+v caveats=%v", row, caveats)
			}
		})
	}
}

func TestGenericStorageUnresolvedRelevantLifecycleResetFailsFamilyClosed(t *testing.T) {
	t.Parallel()
	storagePair := func(resetPID int) *Index {
		return &Index{
			Path: "/trace/bundle.tracebundle.json", TimestampOrder: TraceTimestampOrderMonotonic, LineCount: 3,
			TraceArtifacts: []TraceArtifactSource{
				{SourcePath: "/trace/child-a.systrace", LocalLineCount: 3, VirtualLineBase: 0, CausalCompatible: true},
				// The deliberately overlapping one-row ledger makes only global
				// line 2 provenance-unresolvable while the surrounding endpoints
				// remain uniquely assigned to child A.
				{SourcePath: "/trace/ambiguous-reset.systrace", LocalLineCount: 1, VirtualLineBase: 1, CausalCompatible: true},
			},
			Events: []Event{
				integrityStorageEvent(1, 1.000, 40, "scsi_dispatch_cmd_start", "12,80"),
				{Line: 2, Ts: 1.500, Type: EventSchedSwitch, PrevPID: resetPID, PrevState: "X"},
				integrityStorageEvent(3, 2.000, 40, "scsi_dispatch_cmd_done", "12,80"),
			},
		}
	}
	relevantRows, relevantCaveats := computeStorageLatencyByLayer(storagePair(40), Query{TimeStart: .9, TimeEnd: 2.1}, nil, 8)
	if row := storageLatencyRow(relevantRows, "scsi", "scsi_dispatch_cmd"); row != nil {
		t.Fatalf("unresolved relevant lifecycle reset left storage latency visible: row=%+v caveats=%v", row, relevantCaveats)
	}
	if !containsSubstring(relevantCaveats, "duration_pairing_fail_closed=true") ||
		!containsSubstring(relevantCaveats, "storage_latency_lifecycle_reset_provenance_unresolved=true") {
		t.Fatalf("unresolved relevant lifecycle reset was not explicitly fail-closed: %v", relevantCaveats)
	}

	irrelevantRows, irrelevantCaveats := computeStorageLatencyByLayer(storagePair(99), Query{TimeStart: .9, TimeEnd: 2.1}, nil, 8)
	if row := storageLatencyRow(irrelevantRows, "scsi", "scsi_dispatch_cmd"); row == nil || row.PairedCount != 1 {
		t.Fatalf("unrelated unresolved lifecycle reset erased a proven storage pair: rows=%+v caveats=%v", irrelevantRows, irrelevantCaveats)
	}
	if containsSubstring(irrelevantCaveats, "storage_latency_lifecycle_reset_provenance_unresolved=true") {
		t.Fatalf("unrelated reset acquired storage-family authority: %v", irrelevantCaveats)
	}
}

func TestDurationPairingIntegrityBudgetIsBoundedAndSourcePoisonDominates(t *testing.T) {
	t.Parallel()
	integrity := newDurationPairingIntegrityWithBudget(durationOrderWorkqueue, 2)
	if !integrity.poisonLane("lane-a", "source-a") || !integrity.poisonLane("lane-b", "source-b") {
		t.Fatal("initial bounded lanes were not admitted")
	}
	integrity.poisonLane("lane-c", "source-c")
	if !integrity.familyGlobal || !integrity.budgetExceeded || len(integrity.poisonedLanes) > 2 {
		t.Fatalf("poison map exceeded bounded escalation: %+v", integrity)
	}
	dominated := newDurationPairingIntegrityWithBudget(durationOrderWorkqueue, 2)
	if !dominated.poisonSource("source-a") {
		t.Fatal("source poison was not admitted")
	}
	for i := 0; i < 10; i++ {
		dominated.poisonLane(fmt.Sprintf("lane-a-%d", i), "source-a")
	}
	if dominated.familyGlobal || len(dominated.poisonedLanes) != 0 {
		t.Fatalf("already-poisoned source consumed exact-lane budget: %+v", dominated)
	}
}

func TestCompletePairingReplayBudgetFailsAllHardFamiliesClosed(t *testing.T) {
	t.Parallel()
	families := []durationOrderFamily{durationOrderWorkqueue, durationOrderDMAFence, durationOrderBlockIO, durationOrderStorage}
	integrities := make(map[durationOrderFamily]*durationPairingIntegrity, len(families))
	for _, family := range families {
		integrities[family] = newDurationPairingIntegrity(family)
	}
	if applyDurationPairingReplayBudget(integrities, durationPairingReplayEventBudget) {
		t.Fatal("exact replay budget was rejected")
	}
	if !applyDurationPairingReplayBudget(integrities, durationPairingReplayEventBudget+1) {
		t.Fatal("over-budget replay was admitted")
	}
	for _, family := range families {
		integrity := integrities[family]
		if !integrity.familyGlobal || !integrity.budgetExceeded || integrity.globalWitnesses != 1 ||
			!containsSubstring(integrity.caveats("test"), "budget_exceeded=true") {
			t.Fatalf("family %s did not fail closed at the shared replay cap: %+v", family, integrity)
		}
	}
}

func TestPairingHistoryOverflowDefersToCompleteTimestampProof(t *testing.T) {
	t.Parallel()
	tracker := newDurationOrderTracker()
	tracker.pairingHistoryBudget = 2
	monotonic := []Event{
		integrityWorkqueueEvent(1, 1, 10, "workqueue_execute_start", 1), integrityWorkqueueEvent(2, 2, 10, "workqueue_execute_end", 1),
		integrityWorkqueueEvent(3, 3, 10, "workqueue_execute_start", 2), integrityWorkqueueEvent(4, 4, 10, "workqueue_execute_end", 2),
		integrityWorkqueueEvent(5, 5, 10, "workqueue_execute_start", 3), integrityWorkqueueEvent(6, 6, 10, "workqueue_execute_end", 3),
	}
	for _, ev := range monotonic {
		tracker.observeAll(ev)
	}
	if !tracker.pairingHistoryCapped[durationOrderWorkqueue] || tracker.capped[durationOrderWorkqueue] || !durationEventSequenceMonotonic(monotonic) {
		t.Fatalf("history overflow was not separated from active/global cap: history=%v capped=%v", tracker.pairingHistoryCapped, tracker.capped)
	}
	nonMonotonic := append(append([]Event(nil), monotonic...), integrityWorkqueueEvent(7, .5, 10, "workqueue_execute_start", 4))
	if durationEventSequenceMonotonic(nonMonotonic) {
		t.Fatal("non-monotonic control fixture did not regress")
	}
}
