package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func traceDBRawDMALifecycleTestRecord(
	name string,
	timestamp uint64,
) traceDBRawDMALifecycleRecord {
	return traceDBRawDMALifecycleRecord{
		TimestampNS: timestamp, CPU: 3, HeaderPID: 32788,
		Flags: 1, PreemptCount: 2, Name: name,
		Driver: "display", Timeline: "present",
		Context: 7, Seqno: 9,
	}
}

func traceDBRawDMALifecycleTestInventory(
	rows []traceDBRawDMALifecycleRecord,
) *traceDBSourceNameInventory {
	metrics := map[string]int64{}
	for _, row := range rows {
		metrics["target_"+row.Name+"_records"]++
		metrics["target_"+row.Name+"_body_admitted"]++
	}
	return &traceDBSourceNameInventory{
		RawDecode: TraceDBCoverage{
			Found: true,
			Metadata: map[string]string{
				"decode_state": "strict_target_ledger_complete",
			},
			Metrics: metrics,
		},
		RawDMALifecycle: append(
			[]traceDBRawDMALifecycleRecord(nil), rows...),
	}
}

func TestPublishTraceDBRawDMALifecycleRecoveryPublishesOfficialPointEvents(
	t *testing.T,
) {
	rows := []traceDBRawDMALifecycleRecord{
		traceDBRawDMALifecycleTestRecord(
			"dma_fence_init", 1_000_000),
		traceDBRawDMALifecycleTestRecord(
			"dma_fence_enable_signal", 2_000_000),
		traceDBRawDMALifecycleTestRecord(
			"dma_fence_signaled", 3_000_000),
		traceDBRawDMALifecycleTestRecord(
			"dma_fence_destroy", 4_000_000),
	}
	rows[0].Driver = ""
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := publishTraceDBRawDMALifecycleRecovery(
		context.Background(),
		traceDBRawDMALifecycleTestInventory(rows), sink, nil)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.Role != "query_ready_export" ||
		coverage.RowsRead != 4 || coverage.RowsEmitted != 4 ||
		coverage.Metadata["publication_state"] !=
			"published_exact_official_point_events" ||
		coverage.Metadata["official_semantics"] !=
			"point_event_not_interval" ||
		len(sink.rows) != 4 {
		t.Fatalf("official DMA lifecycle points were not published: coverage=%+v rows=%+v",
			coverage, sink.rows)
	}
	body := ""
	for _, row := range sink.rows {
		body += row.line + "\n"
	}
	for _, want := range []string{
		"unknown-32788 (-----) [003] d..2",
		"dma_fence_init: driver= timeline=present context=7 seqno=9",
		"dma_fence_enable_signal: driver=display timeline=present context=7 seqno=9",
		"dma_fence_signaled: driver=display timeline=present context=7 seqno=9",
		"dma_fence_destroy: driver=display timeline=present context=7 seqno=9",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("official DMA point wire missing %q:\n%s",
				want, body)
		}
	}
	for _, forbidden := range []string{"B|", "E|", "duration=", "dur="} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("DMA lifecycle point acquired interval semantics %q:\n%s",
				forbidden, body)
		}
	}
}

func TestDecodeDirectDMAFenceLifecycleAllowsOnlyExactEmptyDriver(
	t *testing.T,
) {
	emptyDriver := directPairDMAFixture(
		"dma_fence_init", nil, []byte("present"), 7, 9, false)
	event := decodeEvent(emptyDriver.format, emptyDriver.content)
	dma, admission, reason :=
		decodeDirectDMAFenceLifecycleFields(event, emptyDriver.content)
	if admission != bodyAdmitted || reason != "" || dma == nil ||
		!dma.DriverKnown || dma.Driver != "" ||
		!dma.TimelineKnown || dma.Timeline != "present" {
		t.Fatalf("official empty lifecycle driver rejected: admission=%d reason=%q dma=%+v",
			admission, reason, dma)
	}
	if body, ok := renderCanonicalDMAFenceLifecycleFields(dma); !ok ||
		body != "driver= timeline=present context=7 seqno=9" {
		t.Fatalf("official empty lifecycle driver wire=%q ok=%v", body, ok)
	}
	if _, waitAdmission, _ :=
		decodeDirectDMAFenceFields(event, emptyDriver.content); waitAdmission != bodyRejected {
		t.Fatalf("empty lifecycle driver leaked into wait hard-key profile: admission=%d",
			waitAdmission)
	}

	emptyTimeline := directPairDMAFixture(
		"dma_fence_init", []byte("display"), nil, 7, 9, false)
	if _, timelineAdmission, _ := decodeDirectDMAFenceLifecycleFields(
		decodeEvent(emptyTimeline.format, emptyTimeline.content),
		emptyTimeline.content); timelineAdmission != bodyRejected {
		t.Fatalf("empty lifecycle timeline admitted: admission=%d",
			timelineAdmission)
	}
}

func TestPublishTraceDBRawDMALifecycleRecoveryWithdrawalArms(t *testing.T) {
	rows := []traceDBRawDMALifecycleRecord{
		traceDBRawDMALifecycleTestRecord(
			"dma_fence_init", 1_000_000),
	}
	t.Run("raw DB overlap", func(t *testing.T) {
		coverage, err := publishTraceDBRawDMALifecycleRecovery(
			context.Background(),
			traceDBRawDMALifecycleTestInventory(rows), nil,
			[]TraceDBCoverage{{
				Family:      "raw_ftrace",
				Table:       "dma_fence",
				RowsEmitted: 1,
			}})
		if err != nil || coverage.RowsEmitted != 0 ||
			coverage.Metadata["publication_state"] !=
				"withheld_db_raw_dma_overlap" {
			t.Fatalf("raw DB overlap did not withdraw source points: coverage=%+v err=%v",
				coverage, err)
		}
	})
	t.Run("invalid retained point is atomic", func(t *testing.T) {
		bad := append([]traceDBRawDMALifecycleRecord(nil), rows...)
		bad[0].HeaderPID = -1
		sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
		if err != nil {
			t.Fatal(err)
		}
		defer sink.cleanup()
		coverage, err := publishTraceDBRawDMALifecycleRecovery(
			context.Background(),
			traceDBRawDMALifecycleTestInventory(bad), sink, nil)
		if err != nil || coverage.RowsEmitted != 0 ||
			len(sink.rows) != 0 ||
			coverage.Metadata["publication_state"] !=
				"withheld_invalid_point_envelope" {
			t.Fatalf("invalid lifecycle point partially published: coverage=%+v rows=%+v err=%v",
				coverage, sink.rows, err)
		}
	})
}

func TestTraceDBSourceRawDecodeLedgerRetainsStrictDMALifecyclePoints(
	t *testing.T,
) {
	var capture bytes.Buffer
	writeFileHeader(&capture, 2)
	header := capture.Bytes()
	binary.LittleEndian.PutUint16(
		header[0:2], traceStreamerRawTraceMagic)
	header[2] = harmonyRMQFileType
	capture.Reset()
	capture.Write(header)
	fields := []string{
		syntheticField(
			"unsigned short", "common_type", 0, 2, false),
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField(
			"__data_loc char[]", "driver", 8, 4, true),
		syntheticField(
			"__data_loc char[]", "timeline", 12, 4, true),
		syntheticField(
			"unsigned int", "context", 16, 4, false),
		syntheticField(
			"unsigned int", "seqno", 20, 4, false),
	}
	names := []string{
		"dma_fence_destroy",
		"dma_fence_enable_signal",
		"dma_fence_init",
		"dma_fence_signaled",
	}
	var formatLines []string
	var events []syntheticRawEvent
	for index, name := range names {
		id := 33639 + index
		formatLines = append(
			formatLines, syntheticFormatBlock(name, id, fields)...)
		driver := []byte("display")
		if name == "dma_fence_init" {
			driver = nil
		}
		content := directPairDMAContent(
			24, driver, []byte("present"),
			uint32(7+index), uint32(9+index))
		binary.LittleEndian.PutUint16(
			content[0:2], uint16(id))
		content[2], content[3] = 1, 2
		binary.LittleEndian.PutUint32(content[4:8], 32788)
		events = append(events, syntheticRawEvent{
			EventID:  uint16(id),
			OffsetNS: uint32(9 + index),
			Content:  content,
		})
	}
	writeSegment(
		&capture, segmentEventsFormat,
		[]byte(strings.Join(formatLines, "\n")))
	writeSegment(
		&capture, segmentRawTrace,
		syntheticRawPageEvents(events))
	path := filepath.Join(
		t.TempDir(), "official-dma-lifecycle-decode.sys")
	if err := os.WriteFile(
		path, capture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(path)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	inventory, err := scanTraceDBSourceNameInventory(
		context.Background(), authority)
	if err != nil {
		t.Fatal(err)
	}
	decode := inventory.RawDecode
	if decode.Metadata["decode_state"] !=
		"strict_target_ledger_complete" ||
		decode.Metrics["target_body_unsupported"] != 0 ||
		decode.Metrics["target_dma_fence_lifecycle_record_capture_failed"] != 0 ||
		len(inventory.RawDMALifecycle) != 4 {
		t.Fatalf("strict DMA lifecycle retention mismatch: decode=%+v rows=%+v",
			decode, inventory.RawDMALifecycle)
	}
	for index, row := range inventory.RawDMALifecycle {
		expectedDriver := "display"
		if row.Name == "dma_fence_init" {
			expectedDriver = ""
		}
		if row.Name != names[index] || row.HeaderPID != 32788 ||
			row.CPU != 1 || row.Flags != 1 ||
			row.PreemptCount != 2 ||
			row.Driver != expectedDriver ||
			row.Timeline != "present" ||
			row.Context != uint64(7+index) ||
			row.Seqno != uint64(9+index) {
			t.Fatalf("DMA lifecycle row %d mismatch: %+v",
				index, row)
		}
	}
}

func TestTraceStreamerConversionPublishesOfficialRawDMALifecyclePoint(
	t *testing.T,
) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer fixture uses a POSIX shell")
	}
	var capture bytes.Buffer
	writeFileHeader(&capture, 2)
	header := capture.Bytes()
	binary.LittleEndian.PutUint16(
		header[0:2], traceStreamerRawTraceMagic)
	header[2] = harmonyRMQFileType
	capture.Reset()
	capture.Write(header)
	fields := []string{
		syntheticField(
			"unsigned short", "common_type", 0, 2, false),
		syntheticField(
			"unsigned char", "common_flags", 2, 1, false),
		syntheticField(
			"unsigned char", "common_preempt_count", 3, 1, false),
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField(
			"__data_loc char[]", "driver", 8, 4, true),
		syntheticField(
			"__data_loc char[]", "timeline", 12, 4, true),
		syntheticField(
			"unsigned int", "context", 16, 4, false),
		syntheticField(
			"unsigned int", "seqno", 20, 4, false),
	}
	content := directPairDMAContent(
		24, nil, []byte("present"), 7, 9)
	binary.LittleEndian.PutUint16(content[0:2], 33638)
	content[2], content[3] = 1, 2
	binary.LittleEndian.PutUint32(content[4:8], 32788)
	writeSegment(
		&capture, segmentEventsFormat,
		[]byte(strings.Join(
			syntheticFormatBlock(
				"dma_fence_init", 33638, fields), "\n")))
	writeSegment(
		&capture, segmentRawTrace,
		syntheticRawPageEvents([]syntheticRawEvent{{
			EventID: 33638, OffsetNS: 9, Content: content,
		}}))
	dir := t.TempDir()
	input := filepath.Join(dir, "official-dma.sys")
	output := filepath.Join(dir, "official-dma.systrace")
	if err := os.WriteFile(input, capture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(
		t, traceStreamerIntegrationDBStatements())
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: output,
		TraceEngine:       traceEngineTraceStreamer,
		TraceStreamerPath: traceStreamer,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	wire := "dma_fence_init: driver= timeline=present context=7 seqno=9"
	if strings.Count(string(body), wire) != 1 {
		t.Fatalf("conversion wiring did not publish exactly one official DMA point:\n%s",
			body)
	}
	for _, coverage := range result.TraceDBCoverage {
		if coverage.Family != "source_rawtrace_dma_lifecycle" {
			continue
		}
		if coverage.RowsRead != 1 || coverage.RowsEmitted != 1 ||
			coverage.Metadata["publication_state"] !=
				"published_exact_official_point_events" {
			t.Fatalf("DMA lifecycle conversion coverage mismatch: %+v",
				coverage)
		}
		return
	}
	t.Fatal("DMA lifecycle conversion wiring coverage missing")
}
