package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func traceDBRawBlockRecoveryCapture(t *testing.T) []byte {
	t.Helper()
	values := directBlockFixtureValues{
		dev: uint64(12<<20 | 80), sector: 923339752, nrSector: 64,
		bytes: 32768, rwbs: "RCVHS", comm: "com.tencent.mm",
	}
	issue := directBlockPairFixture("block_rq_issue", 25827, values)
	complete := directBlockPairFixture("block_rq_complete", 2, directBlockFixtureValues{
		dev: values.dev, sector: values.sector, nrSector: values.nrSector,
		rwbs: values.rwbs,
	})

	var capture bytes.Buffer
	writeFileHeader(&capture, 2)
	header := capture.Bytes()
	binary.LittleEndian.PutUint16(header[0:2], traceStreamerRawTraceMagic)
	header[2] = harmonyRMQFileType
	capture.Reset()
	capture.Write(header)
	writeSegment(&capture, segmentCmdlines, []byte("25827 com.tencent.mm\n2 udk-irq-4-80\n"))
	writeSegment(&capture, segmentEventsFormat, []byte(strings.Join([]string{
		directPairFormatBlock(33642, issue.format),
		directPairFormatBlock(33643, complete.format),
	}, "\n")))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 33642, OffsetNS: 1000, Content: issue.content},
		{EventID: 33643, OffsetNS: 276000, Content: complete.content},
	}))
	return capture.Bytes()
}

func TestTraceDBSourceRawBlockRecoveryRetainsAndPublishesExactCrossEmitterPair(t *testing.T) {
	path := filepath.Join(t.TempDir(), "official-block.sys")
	if err := os.WriteFile(path, traceDBRawBlockRecoveryCapture(t), 0o600); err != nil {
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
	if !traceDBRawBlockFamilyAuthorityEligible(&inventory) ||
		inventory.RawDecode.Metadata["retention_block_storage_state"] != "complete" ||
		inventory.RawDecode.Metrics["target_block_rq_issue_body_admitted"] != 1 ||
		inventory.RawDecode.Metrics["target_block_rq_complete_body_admitted"] != 1 ||
		len(inventory.RawBlock) != 2 {
		t.Fatalf("strict source block family was not retained: decode=%+v rows=%+v",
			inventory.RawDecode, inventory.RawBlock)
	}
	if inventory.RawBlock[0].HeaderPID != 25827 || inventory.RawBlock[0].CPU != 1 ||
		inventory.RawBlock[0].Body != "12,80 RCVHS 32768 () 923339752 + 64 [com.tencent.mm]" ||
		inventory.RawBlock[1].HeaderPID != 2 ||
		inventory.RawBlock[1].Body != "12,80 RCVHS () 923339752 + 64 [0]" {
		t.Fatalf("retained source block envelope/body drifted: %+v", inventory.RawBlock)
	}

	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := publishTraceDBRawBlockRecovery(
		context.Background(), &inventory, sink, nil)
	if err != nil {
		t.Fatal(err)
	}
	if coverage.RowsRead != 2 || coverage.RowsEmitted != 2 ||
		coverage.Metadata["publication_state"] != "published_complete_exact_source_family" ||
		coverage.Metrics["pairing_endpoint_rows_published"] != 2 || len(sink.rows) != 2 {
		t.Fatalf("source block pair publication mismatch: coverage=%+v rows=%+v",
			coverage, sink.rows)
	}
	body := sink.rows[0].line + "\n" + sink.rows[1].line + "\n"
	for _, want := range []string{
		"com.tencent.mm-25827 (-----) [001]",
		"udk-irq-4-80-2     (-----) [001]",
		"block_rq_issue: 12,80 RCVHS 32768 () 923339752 + 64 [com.tencent.mm]",
		"block_rq_complete: 12,80 RCVHS () 923339752 + 64 [0]",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("source block wire missing %q:\n%s", want, body)
		}
	}
	systrace := filepath.Join(t.TempDir(), "source-block.systrace")
	if err := os.WriteFile(systrace, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := tracequery.BuildIndex(context.Background(), systrace)
	if err != nil {
		t.Fatal(err)
	}
	stats := tracequery.ComputeWindowStats(index, tracequery.Query{})
	if len(stats.IOLatencies) != 1 ||
		stats.IOLatencies[0].IssueThread.PID != 25827 ||
		stats.IOLatencies[0].CompleteThread.PID != 2 ||
		math.Abs(stats.IOLatencies[0].DurationMs-0.275) > 1e-6 {
		t.Fatalf("recovered source block pair did not construct exact request residence: %+v caveats=%v",
			stats.IOLatencies, stats.Caveats)
	}
}

func TestPublishTraceDBRawBlockRecoveryWithdrawalAndSourcePrecedence(t *testing.T) {
	rows := []traceDBRawBlockRecord{
		{PhysicalOrdinal: 1, TimestampNS: 1_000_000, CPU: 1, HeaderPID: 10,
			Name: "block_rq_issue", Body: "8,0 R 4096 () 100 + 8 [io]"},
		{PhysicalOrdinal: 2, TimestampNS: 2_000_000, CPU: 2, HeaderPID: 2,
			Name: "block_rq_complete", Body: "8,0 R () 100 + 8 [0]"},
	}
	inventory := &traceDBSourceNameInventory{
		RawDecode: TraceDBCoverage{
			Found: true,
			Metadata: map[string]string{
				"decode_state":                  "strict_target_ledger_complete",
				"retention_block_storage_state": "complete",
			},
			Metrics: map[string]int64{
				"target_block_rq_issue_records":              1,
				"target_block_rq_issue_body_admitted":        1,
				"target_block_rq_complete_records":           1,
				"target_block_rq_complete_body_admitted":     1,
				"target_block_storage_records_retained":      2,
				"target_block_storage_record_capture_failed": 0,
			},
		},
		RawBlock: rows,
	}
	// EVOLUTION RECORD (§40.12 V6-1): the overlap witness used to be the
	// class-wide block_storage RowsEmitted, which also counted MMC/UFS/SCSI
	// rows no source family governs. The overlap now reads the exact
	// governed-name publishable census; a class-wide RowsEmitted with zero
	// governed rows is not an overlap.
	coverage, err := publishTraceDBRawBlockRecovery(
		context.Background(), inventory, nil, []TraceDBCoverage{{
			Family: "raw_ftrace", Table: "block_storage", RowsEmitted: 1,
			Metrics: map[string]int64{"source_governed_block_storage_rows_publishable": 1},
		}})
	if err != nil || coverage.RowsEmitted != 0 ||
		coverage.Metrics["db_raw_governed_block_rows_publishable"] != 1 ||
		coverage.Metadata["publication_state"] != "withheld_db_raw_block_overlap" {
		t.Fatalf("DB/source overlap did not fail closed: coverage=%+v err=%v", coverage, err)
	}
	ungoverned, err := publishTraceDBRawBlockRecovery(
		context.Background(), inventory, nil, []TraceDBCoverage{{
			Family: "raw_ftrace", Table: "block_storage", RowsEmitted: 4,
		}})
	if err == nil || ungoverned.Metadata["publication_state"] == "withheld_db_raw_block_overlap" {
		t.Fatalf("class-wide block_storage rows without governed names were read as a block overlap: coverage=%+v err=%v",
			ungoverned, err)
	}

	bad := *inventory
	bad.RawBlock = append([]traceDBRawBlockRecord(nil), rows...)
	bad.RawBlock[1].PhysicalOrdinal = 1
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err = publishTraceDBRawBlockRecovery(
		context.Background(), &bad, sink, nil)
	if err != nil || coverage.RowsEmitted != 0 || len(sink.rows) != 0 ||
		coverage.Metadata["publication_state"] != "withheld_invalid_retained_envelope" {
		t.Fatalf("duplicate physical ordinal partially published: coverage=%+v rows=%+v err=%v",
			coverage, sink.rows, err)
	}
}

func TestCompleteSourceRawBlockFamilySupersedesSQLiteWithoutPoisoningStage(t *testing.T) {
	path := createTraceDBRawAuthorityFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 10, 'app')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 10, 1, 'io', 0, 1, 1)",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'dev')",
		"INSERT INTO data_dict VALUES (2, 'sector')",
		"INSERT INTO data_dict VALUES (3, 'nr_sector')",
		"INSERT INTO data_dict VALUES (4, 'bytes')",
		"INSERT INTO data_dict VALUES (5, 'rwbs')",
		"INSERT INTO data_dict VALUES (6, 'cmd')",
		"INSERT INTO data_dict VALUES (7, 'comm')",
		"INSERT INTO data_dict VALUES (8, 'error')",
		"INSERT INTO data_dict VALUES (10, '8,0')",
		"INSERT INTO data_dict VALUES (11, 'R')",
		"INSERT INTO data_dict VALUES (12, 'READ')",
		"INSERT INTO data_dict VALUES (13, 'io')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (1, 1, 1, 10)",
		"INSERT INTO args VALUES (1, 2, 0, 100)",
		"INSERT INTO args VALUES (1, 3, 0, 8)",
		"INSERT INTO args VALUES (1, 4, 0, 4096)",
		"INSERT INTO args VALUES (1, 5, 1, 11)",
		"INSERT INTO args VALUES (1, 6, 1, 12)",
		"INSERT INTO args VALUES (1, 7, 1, 13)",
		"INSERT INTO args VALUES (2, 1, 1, 10)",
		"INSERT INTO args VALUES (2, 2, 0, 100)",
		"INSERT INTO args VALUES (2, 3, 0, 8)",
		"INSERT INTO args VALUES (2, 5, 1, 11)",
		"INSERT INTO args VALUES (2, 6, 1, 12)",
		"INSERT INTO args VALUES (2, 8, 0, 0)",
		"CREATE TABLE raw (id, ts, name, cpu, itid, argsetid)",
		"INSERT INTO raw VALUES (1, 1000000, 'block_rq_issue', 1, 1, 1)",
		"INSERT INTO raw VALUES (2, 2000000, 'block_rq_complete', 2, 1, 2)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	rows := []traceDBRawBlockRecord{
		{PhysicalOrdinal: 1, TimestampNS: 1_000_000, CPU: 1, HeaderPID: 10,
			Name: "block_rq_issue", Body: "8,0 R 4096 (READ) 100 + 8 [io]"},
		{PhysicalOrdinal: 2, TimestampNS: 2_000_000, CPU: 2, HeaderPID: 10,
			Name: "block_rq_complete", Body: "8,0 R (READ) 100 + 8 [0]"},
	}
	tdb.sourceNameInventory = &traceDBSourceNameInventory{
		Names: map[int64]string{10: "io"},
		RawDecode: TraceDBCoverage{
			Found: true,
			Metadata: map[string]string{
				"decode_state":                  "strict_target_ledger_complete",
				"retention_block_storage_state": "complete",
			},
			Metrics: map[string]int64{
				"target_block_rq_issue_records":              1,
				"target_block_rq_issue_body_admitted":        1,
				"target_block_rq_complete_records":           1,
				"target_block_rq_complete_body_admitted":     1,
				"target_block_storage_records_retained":      2,
				"target_block_storage_record_capture_failed": 0,
			},
		},
		RawBlock: rows,
	}
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 10, Name: "app"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 10, IPID: 1, Name: "io"}
	buildTraceDBThreadSecondaryIndexes(&index)
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	rawCoverage, err := exportTraceDBRawFtraceFamilies(
		context.Background(), tdb, sink, traceDBTestCompleteSchedulerAuthority(index),
		traceDBSchedulerRunningIndex{}, filepath.Join(t.TempDir(), "source-precedence.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	var schema, block TraceDBCoverage
	for _, item := range rawCoverage {
		if item.Family == "raw_ftrace" && item.Table == "raw" {
			schema = item
		}
		if item.Family == "raw_ftrace" && item.Table == "block_storage" {
			block = item
		}
	}
	if block.RowsRead != 2 || block.RowsEmitted != 0 ||
		!strings.Contains(block.Skipped, "superseded_complete_source_raw_block_family=2") ||
		!strings.Contains(schema.FieldSources["pairing_stage_backend"], "poisoned_lanes=0") ||
		len(sink.rows) != 0 {
		t.Fatalf("complete source family did not cleanly supersede DB block rows: schema=%+v block=%+v rows=%+v",
			schema, block, sink.rows)
	}
	recovered, err := publishTraceDBRawBlockRecovery(
		context.Background(), tdb.sourceNameInventory, sink, rawCoverage)
	if err != nil || recovered.RowsEmitted != 2 || len(sink.rows) != 2 {
		t.Fatalf("source authority did not replace suppressed DB rows: coverage=%+v rows=%+v err=%v",
			recovered, sink.rows, err)
	}
}

func TestTraceStreamerConversionPublishesOfficialSourceRawBlockFamily(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer fixture uses a POSIX shell")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "official-block.sys")
	output := filepath.Join(dir, "official-block.systrace")
	if err := os.WriteFile(input, traceDBRawBlockRecoveryCapture(t), 0o600); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: output,
		TraceEngine: traceEngineTraceStreamer, TraceStreamerPath: traceStreamer,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), "block_rq_issue: 12,80 RCVHS 32768 () 923339752 + 64 [com.tencent.mm]") != 1 ||
		strings.Count(string(body), "block_rq_complete: 12,80 RCVHS () 923339752 + 64 [0]") != 1 {
		t.Fatalf("trace_streamer normalization did not recover exactly one source block pair:\n%s", body)
	}
	found := false
	for _, coverage := range result.TraceDBCoverage {
		if coverage.Family == "source_rawtrace_block" {
			found = coverage.RowsEmitted == 2 &&
				coverage.Metadata["publication_state"] == "published_complete_exact_source_family"
		}
	}
	if !found {
		t.Fatalf("source block recovery coverage missing: %+v", result.TraceDBCoverage)
	}
}
