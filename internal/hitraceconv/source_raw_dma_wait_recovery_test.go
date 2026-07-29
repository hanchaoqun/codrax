package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func traceDBRawDMAWaitTestRecord(name string, ts uint64, seqno uint64) traceDBRawDMAWaitRecord {
	return traceDBRawDMAWaitRecord{
		TimestampNS: ts, CPU: 3, HeaderPID: 201,
		Flags: 1, PreemptCount: 2, Name: name,
		Driver: "display", Timeline: "present", Context: 7, Seqno: seqno,
	}
}

func traceDBRawDMAWaitTestInventory(rows []traceDBRawDMAWaitRecord) *traceDBSourceNameInventory {
	starts, ends := int64(0), int64(0)
	for _, row := range rows {
		switch row.Name {
		case "dma_fence_wait_start":
			starts++
		case "dma_fence_wait_end":
			ends++
		}
	}
	return &traceDBSourceNameInventory{
		RawDecode: TraceDBCoverage{
			Found: true,
			Metadata: map[string]string{
				"decode_state": "strict_target_ledger_complete",
			},
			Metrics: map[string]int64{
				"target_dma_fence_wait_start_records":       starts,
				"target_dma_fence_wait_end_records":         ends,
				"target_dma_fence_wait_start_body_admitted": starts,
				"target_dma_fence_wait_end_body_admitted":   ends,
			},
		},
		RawDMAWait: append([]traceDBRawDMAWaitRecord(nil), rows...),
	}
}

func TestPublishTraceDBRawDMAWaitRecoveryPublishesExactCleanPair(t *testing.T) {
	rows := []traceDBRawDMAWaitRecord{
		traceDBRawDMAWaitTestRecord("dma_fence_wait_start", 1_000_000, 9),
		traceDBRawDMAWaitTestRecord("dma_fence_wait_end", 4_000_000, 9),
	}
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sink.cleanup(); err != nil {
			t.Errorf("cleanup raw DMA wait sink: %v", err)
		}
	}()
	coverage, err := publishTraceDBRawDMAWaitRecovery(
		context.Background(), traceDBRawDMAWaitTestInventory(rows), sink,
		traceDBRawBlockedKeyTestAuthority(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Role != "query_ready_export" || coverage.RowsRead != 2 ||
		coverage.RowsEmitted != 2 ||
		coverage.Metadata["publication_state"] != "published_exact_clean_pair_lanes" ||
		coverage.Metrics["pairs_published"] != 1 ||
		coverage.Metrics["pair_lanes_published"] != 1 ||
		len(sink.rows) != 2 {
		t.Fatalf("clean DMA wait pair was not recovered: coverage=%+v rows=%+v",
			coverage, sink.rows)
	}
	body := sink.rows[0].line + "\n" + sink.rows[1].line
	for _, want := range []string{
		"header-thread-201", "(  200) [003] d..2",
		"dma_fence_wait_start: driver=display timeline=present context=7 seqno=9",
		"dma_fence_wait_end: driver=display timeline=present context=7 seqno=9",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("exact DMA wait wire missing %q:\n%s", want, body)
		}
	}
	path := filepath.Join(t.TempDir(), "raw-dma-wait.systrace")
	if err := os.WriteFile(path, []byte(body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	stats := tracequery.ComputeWindowStats(index, tracequery.Query{})
	if len(stats.DMAFenceActivity) != 1 ||
		stats.DMAFenceActivity[0].PairedCount != 1 ||
		stats.DMAFenceActivity[0].WaitMs != 3 {
		t.Fatalf("tracequery did not consume recovered DMA wait pair: %+v", stats.DMAFenceActivity)
	}
}

func TestPublishTraceDBRawDMAWaitRecoveryFailsClosedWithoutPoisoningCleanSibling(t *testing.T) {
	rows := []traceDBRawDMAWaitRecord{
		traceDBRawDMAWaitTestRecord("dma_fence_wait_start", 1_000_000, 9),
		traceDBRawDMAWaitTestRecord("dma_fence_wait_start", 2_000_000, 9),
		traceDBRawDMAWaitTestRecord("dma_fence_wait_end", 3_000_000, 9),
		traceDBRawDMAWaitTestRecord("dma_fence_wait_start", 4_000_000, 10),
		traceDBRawDMAWaitTestRecord("dma_fence_wait_end", 8_000_000, 10),
	}
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := publishTraceDBRawDMAWaitRecovery(
		context.Background(), traceDBRawDMAWaitTestInventory(rows), sink,
		traceDBRawBlockedKeyTestAuthority(), nil)
	if err != nil {
		t.Fatal(err)
	}
	body := ""
	for _, row := range sink.rows {
		body += row.line + "\n"
	}
	if coverage.RowsEmitted != 2 ||
		coverage.Metrics["pair_lanes_poisoned"] != 1 ||
		coverage.Metrics["raw_endpoints_withheld_poisoned_lane"] != 3 ||
		strings.Contains(body, "seqno=9") ||
		strings.Count(body, "seqno=10") != 2 {
		t.Fatalf("poisoned lane bridged a hole or clean sibling was lost: coverage=%+v body=%s",
			coverage, body)
	}
}

func TestPublishTraceDBRawDMAWaitRecoveryTypedWithdrawalArms(t *testing.T) {
	base := []traceDBRawDMAWaitRecord{
		traceDBRawDMAWaitTestRecord("dma_fence_wait_start", 1_000_000, 9),
		traceDBRawDMAWaitTestRecord("dma_fence_wait_end", 4_000_000, 9),
	}
	t.Run("namespace pid unresolved poisons exact lane", func(t *testing.T) {
		rows := append([]traceDBRawDMAWaitRecord(nil), base...)
		rows[0].HeaderPID, rows[1].HeaderPID = 32788, 32788
		coverage, err := publishTraceDBRawDMAWaitRecovery(
			context.Background(), traceDBRawDMAWaitTestInventory(rows), nil,
			traceDBRawBlockedKeyTestAuthority(), nil)
		if err != nil || coverage.RowsEmitted != 0 ||
			coverage.Metadata["publication_state"] != "complete_no_clean_pair_lane" ||
			coverage.Metrics["pair_lanes_poisoned"] != 1 {
			t.Fatalf("namespace-shaped PID gained a host rewrite: coverage=%+v err=%v", coverage, err)
		}
	})
	t.Run("DB raw DMA overlap withdraws source family", func(t *testing.T) {
		coverage, err := publishTraceDBRawDMAWaitRecovery(
			context.Background(), traceDBRawDMAWaitTestInventory(base), nil,
			traceDBRawBlockedKeyTestAuthority(), []TraceDBCoverage{{
				Family: "raw_ftrace", Table: "dma_fence", RowsEmitted: 1,
			}})
		if err != nil || coverage.RowsEmitted != 0 ||
			coverage.Metadata["publication_state"] != "withheld_db_raw_dma_overlap" {
			t.Fatalf("DB/source raw overlap was not failed closed: coverage=%+v err=%v", coverage, err)
		}
	})
	t.Run("sub-microsecond physical interval remains exact", func(t *testing.T) {
		rows := append([]traceDBRawDMAWaitRecord(nil), base...)
		rows[1].TimestampNS = rows[0].TimestampNS + 999
		sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		coverage, err := publishTraceDBRawDMAWaitRecovery(
			context.Background(), traceDBRawDMAWaitTestInventory(rows), sink,
			traceDBRawBlockedKeyTestAuthority(), nil)
		if err != nil || coverage.RowsEmitted != 2 ||
			coverage.Metadata["publication_state"] != "published_exact_clean_pair_lanes" ||
			coverage.Metrics["pairs_published"] != 1 ||
			len(sink.rows) != 2 ||
			!strings.Contains(sink.rows[0].line, "0.001000:") ||
			!strings.Contains(sink.rows[1].line, "0.001000999:") {
			t.Fatalf("exact sub-microsecond wait was not published: coverage=%+v rows=%+v err=%v",
				coverage, sink.rows, err)
		}
	})
	t.Run("retained census mismatch", func(t *testing.T) {
		inventory := traceDBRawDMAWaitTestInventory(base)
		inventory.RawDMAWait = inventory.RawDMAWait[:1]
		coverage, err := publishTraceDBRawDMAWaitRecovery(
			context.Background(), inventory, nil,
			traceDBRawBlockedKeyTestAuthority(), nil)
		if err != nil || coverage.RowsEmitted != 0 ||
			coverage.Metadata["publication_state"] != "withheld_raw_endpoint_census_incomplete" {
			t.Fatalf("partial retained census gained publication: coverage=%+v err=%v", coverage, err)
		}
	})
	t.Run("cancelled before publication", func(t *testing.T) {
		sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		coverage, err := publishTraceDBRawDMAWaitRecovery(
			ctx, traceDBRawDMAWaitTestInventory(base), sink,
			traceDBRawBlockedKeyTestAuthority(), nil)
		if !errors.Is(err, context.Canceled) || coverage.RowsEmitted != 0 ||
			sink.stats.RowsAccepted != 0 {
			t.Fatalf("cancelled DMA recovery partially published: coverage=%+v err=%v", coverage, err)
		}
	})
}

func TestTraceDBSourceRawDecodeLedgerRetainsStrictDMAWaitEndpoints(t *testing.T) {
	var capture bytes.Buffer
	writeFileHeader(&capture, 2)
	header := capture.Bytes()
	binary.LittleEndian.PutUint16(header[0:2], traceStreamerRawTraceMagic)
	header[2] = harmonyRMQFileType
	capture.Reset()
	capture.Write(header)
	fields := []string{
		syntheticField("unsigned short", "common_type", 0, 2, false),
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("__data_loc char[]", "driver", 8, 4, true),
		syntheticField("__data_loc char[]", "timeline", 12, 4, true),
		syntheticField("unsigned int", "context", 16, 4, false),
		syntheticField("unsigned int", "seqno", 20, 4, false),
	}
	format := strings.Join(append(
		syntheticFormatBlock("dma_fence_wait_start", 33642, fields),
		syntheticFormatBlock("dma_fence_wait_end", 33643, fields)...,
	), "\n")
	start := directPairDMAContent(24, []byte("display"), []byte("present"), 7, 9)
	end := directPairDMAContent(24, []byte("display"), []byte("present"), 7, 9)
	binary.LittleEndian.PutUint16(start[0:2], 33642)
	binary.LittleEndian.PutUint16(end[0:2], 33643)
	start[2], start[3], end[2], end[3] = 1, 2, 3, 4
	binary.LittleEndian.PutUint32(start[4:8], 201)
	binary.LittleEndian.PutUint32(end[4:8], 201)
	writeSegment(&capture, segmentEventsFormat, []byte(format))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 33642, OffsetNS: 9, Content: start},
		{EventID: 33643, OffsetNS: 20_009, Content: end},
	}))
	path := filepath.Join(t.TempDir(), "official-dma-wait-decode.sys")
	if err := os.WriteFile(path, capture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(path)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	inventory, err := scanTraceDBSourceNameInventory(context.Background(), authority)
	if err != nil {
		t.Fatal(err)
	}
	decode := inventory.RawDecode
	if decode.Metadata["decode_state"] != "strict_target_ledger_complete" ||
		decode.Metrics["target_dma_fence_wait_start_body_admitted"] != 1 ||
		decode.Metrics["target_dma_fence_wait_end_body_admitted"] != 1 ||
		len(inventory.RawDMAWait) != 2 ||
		inventory.RawDMAWait[0].HeaderPID != 201 ||
		inventory.RawDMAWait[0].Flags != 1 ||
		inventory.RawDMAWait[0].PreemptCount != 2 ||
		inventory.RawDMAWait[0].Driver != "display" ||
		inventory.RawDMAWait[0].Timeline != "present" ||
		inventory.RawDMAWait[0].Context != 7 ||
		inventory.RawDMAWait[0].Seqno != 9 ||
		inventory.RawDMAWait[1].Name != "dma_fence_wait_end" {
		t.Fatalf("strict DMA wait retention mismatch: decode=%+v rows=%+v",
			decode, inventory.RawDMAWait)
	}
}
