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

func traceDBRawExactRecoveryCapture(t *testing.T) []byte {
	t.Helper()
	wqStart := directPairWorkqueueFixture(
		"workqueue_execute_start", 8, true, 0xaaa, 0x111)
	wqEnd := directPairWorkqueueFixture(
		"workqueue_execute_end", 8, true, 0xaaa, 0x222)
	pageAdd := directPageCacheFixture("mm_filemap_add_to_page_cache", 8, false)
	f2fsStart := directF2FSTestFixtureFor(directF2FSProfileWriteBegin66, 8)
	f2fsEnd := directF2FSTestFixtureFor(directF2FSProfileWriteEnd, 8)

	type exactFixture struct {
		id      uint16
		format  eventFormat
		content []byte
	}
	fixtures := []exactFixture{
		{34001, wqStart.format, wqStart.content},
		{34002, wqEnd.format, wqEnd.content},
		{34003, pageAdd.format, pageAdd.content},
		{34004, f2fsStart.format, f2fsStart.content},
		{34005, f2fsEnd.format, f2fsEnd.content},
	}
	formatBlocks := make([]string, 0, len(fixtures))
	events := make([]syntheticRawEvent, 0, len(fixtures))
	for index := range fixtures {
		fixture := &fixtures[index]
		fixture.format.ID = int(fixture.id)
		binary.LittleEndian.PutUint16(fixture.content[:2], fixture.id)
		switch {
		case directF2FSNameGoverned(fixture.format.Name):
			formatBlocks = append(formatBlocks,
				strings.Join(directF2FSSyntheticFormatBlock(fixture.format), "\n"))
		default:
			formatBlocks = append(formatBlocks,
				directPairFormatBlock(int(fixture.id), fixture.format))
		}
		events = append(events, syntheticRawEvent{
			EventID: fixture.id, OffsetNS: uint32(index * 1_000_000), Content: fixture.content,
		})
	}

	var capture bytes.Buffer
	writeFileHeader(&capture, 4)
	header := capture.Bytes()
	binary.LittleEndian.PutUint16(header[0:2], traceStreamerRawTraceMagic)
	header[2] = harmonyRMQFileType
	capture.Reset()
	capture.Write(header)
	writeSegment(&capture, segmentCmdlines, []byte("100 exact-worker\n"))
	writeSegment(&capture, segmentEventsFormat, []byte(strings.Join(formatBlocks, "\n")))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents(events))
	return capture.Bytes()
}

func TestTraceDBSourceRawExactFamiliesRetainAndPublish(t *testing.T) {
	path := filepath.Join(t.TempDir(), "official-exact.sys")
	if err := os.WriteFile(path, traceDBRawExactRecoveryCapture(t), 0o600); err != nil {
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
	for family, want := range map[string]int{
		traceDBRawRetentionWorkqueue: 2,
		traceDBRawRetentionFilemap:   1,
		traceDBRawRetentionF2FS:      2,
	} {
		if !traceDBRawExactFamilyAuthorityEligible(&inventory, family) ||
			inventory.RawDecode.Metadata["retention_"+family+"_state"] != "complete" {
			t.Fatalf("exact source family %s was not retained: decode=%+v rows=%+v",
				family, inventory.RawDecode, inventory.RawExact)
		}
		count := 0
		for _, record := range inventory.RawExact {
			if record.Family == family {
				count++
			}
		}
		if count != want {
			t.Fatalf("exact source family %s retained=%d want=%d", family, count, want)
		}
	}

	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	coverage, err := publishTraceDBRawExactRecoveries(
		context.Background(), &inventory, sink, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(coverage) != 3 || len(sink.rows) != 5 {
		t.Fatalf("exact source family publication mismatch: coverage=%+v rows=%+v",
			coverage, sink.rows)
	}
	for _, item := range coverage {
		if item.Metadata["publication_state"] != "published_complete_exact_source_family" ||
			item.RowsEmitted == 0 {
			t.Fatalf("exact source family was not published completely: %+v", item)
		}
	}
	body := ""
	for _, row := range sink.rows {
		body += row.line + "\n"
	}
	for _, want := range []string{
		"exact-worker-100   (-----)",
		"workqueue_execute_start: work struct 0xaaa: function 0x111",
		"workqueue_execute_end: work struct 0xaaa: function 0x222",
		"mm_filemap_add_to_page_cache: dev 12:48 ino 0x1234 pfn=77 ofs=4096",
		"f2fs_write_begin:",
		"f2fs_write_end:",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("exact source publication missing %q:\n%s", want, body)
		}
	}
}

func TestTraceDBRawExactSourcePrecedenceClassMapping(t *testing.T) {
	for family, wantClass := range map[string]string{
		traceDBRawRetentionWorkqueue: "workqueue",
		traceDBRawRetentionFilemap:   "page_cache",
		traceDBRawRetentionF2FS:      "",
	} {
		if got := traceDBRawExactRecoveryDBClass(family); got != wantClass {
			t.Fatalf("family %s DB class=%q want=%q", family, got, wantClass)
		}
	}
}

func TestCompleteExactSourceFamiliesSupersedeSQLiteWithoutPairPoison(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "official-exact.sys")
	if err := os.WriteFile(sourcePath, traceDBRawExactRecoveryCapture(t), 0o600); err != nil {
		t.Fatal(err)
	}
	authorityView, err := openConversionInputAuthority(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := scanTraceDBSourceNameInventory(context.Background(), authorityView)
	authorityView.Close()
	if err != nil {
		t.Fatal(err)
	}
	dbPath := createTraceDBRawAuthorityFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'app')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 100, 1, 'exact-worker', 0, 1, 1)",
		"CREATE TABLE data_dict (id, data)",
		"INSERT INTO data_dict VALUES (1, 'work')",
		"INSERT INTO data_dict VALUES (2, 'function')",
		"INSERT INTO data_dict VALUES (3, 's_dev')",
		"INSERT INTO data_dict VALUES (4, 'i_ino')",
		"INSERT INTO data_dict VALUES (5, 'index')",
		"INSERT INTO data_dict VALUES (6, 'pfn')",
		"CREATE TABLE args (argset, key, datatype, value)",
		"INSERT INTO args VALUES (1, 1, 0, 2730)",
		"INSERT INTO args VALUES (1, 2, 0, 273)",
		"INSERT INTO args VALUES (2, 1, 0, 2730)",
		"INSERT INTO args VALUES (2, 2, 0, 546)",
		"INSERT INTO args VALUES (3, 3, 0, 12582960)",
		"INSERT INTO args VALUES (3, 4, 0, 4660)",
		"INSERT INTO args VALUES (3, 5, 0, 1)",
		"INSERT INTO args VALUES (3, 6, 0, 77)",
		"CREATE TABLE raw (id, ts, name, cpu, itid, argsetid)",
		"INSERT INTO raw VALUES (1, 1000000, 'workqueue_execute_start', 1, 1, 1)",
		"INSERT INTO raw VALUES (2, 2000000, 'workqueue_execute_end', 1, 1, 2)",
		"INSERT INTO raw VALUES (3, 3000000, 'mm_filemap_add_to_page_cache', 1, 1, 3)",
	})
	tdb, err := openTraceDB(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	tdb.sourceNameInventory = &inventory
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 100, Name: "app"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 100, IPID: 1, Name: "exact-worker"}
	buildTraceDBThreadSecondaryIndexes(&index)
	sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	rawCoverage, err := exportTraceDBRawFtraceFamilies(
		context.Background(), tdb, sink, traceDBTestCompleteSchedulerAuthority(index),
		traceDBSchedulerRunningIndex{}, filepath.Join(t.TempDir(), "source-exact.systrace"))
	if err != nil {
		t.Fatal(err)
	}
	var schema TraceDBCoverage
	for _, item := range rawCoverage {
		if item.Family == "raw_ftrace" && item.Table == "raw" {
			schema = item
		}
	}
	if len(sink.rows) != 0 ||
		!strings.Contains(schema.FieldSources["pairing_stage_backend"], "poisoned_lanes=0") {
		t.Fatalf("source precedence leaked DB rows or poisoned pairing: schema=%+v rows=%+v",
			schema, sink.rows)
	}
	for family, reason := range map[string]string{
		"workqueue":  "superseded_complete_source_raw_workqueue_family=2",
		"page_cache": "superseded_complete_source_raw_filemap_family=1",
	} {
		found := false
		for _, item := range rawCoverage {
			if item.Family == "raw_ftrace" && item.Table == family &&
				strings.Contains(item.Skipped, reason) {
				found = true
			}
		}
		if !found {
			t.Fatalf("raw class %s missing source precedence reason %q: %+v",
				family, reason, rawCoverage)
		}
	}
	published, err := publishTraceDBRawExactRecoveries(
		context.Background(), &inventory, sink, rawCoverage)
	if err != nil || len(sink.rows) != 5 {
		t.Fatalf("source exact families did not replace suppressed DB rows: coverage=%+v rows=%+v err=%v",
			published, sink.rows, err)
	}
}

func TestTraceDBRawHMFSSchemaWitnessIsExactBoundedAndNonAuthoritative(t *testing.T) {
	catalog := eventFormatCatalog{Formats: map[int]eventFormat{
		33086: {
			ID: 33086, Name: "hmfs_writepage", PrintFmt: `"ino=%lu index=%lu", REC->ino, REC->index`,
			Fields: []eventField{
				{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
				{Type: "unsigned long", Name: "ino", Offset: 8, Size: 8},
				{Type: "pgoff_t", Name: "index", Offset: 16, Size: 8},
			},
		},
		33087: {ID: 33087, Name: "hmfs_writepage_vendor", PrintFmt: `"forged"`},
	}}
	geometry, omitted := traceDBRawDecodeTypedGeometryWitnesses(catalog, traceDBRawHMFSFormat)
	prints := traceDBRawHMFSPrintFormatWitnesses(catalog)
	if omitted != 0 || len(geometry) != 1 || len(prints) != 1 ||
		!strings.Contains(geometry[0], "hmfs_writepage#33086") ||
		!strings.Contains(geometry[0], "index@16:8:signed=false:type=pgoff_t") ||
		!strings.Contains(prints[0], "hmfs_writepage#33086/bytes=") ||
		traceDBRawProbeTargetFormat("hmfs_writepage") ||
		traceDBRawDecodeStrictTarget("hmfs_writepage") {
		t.Fatalf("HMFS schema witness gained authority or lost exact evidence: geometry=%v prints=%v omitted=%d",
			geometry, prints, omitted)
	}
}

func TestTraceStreamerConversionPublishesExactSourceFamilies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer fixture uses a POSIX shell")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "official-exact.sys")
	output := filepath.Join(dir, "official-exact.systrace")
	if err := os.WriteFile(input, traceDBRawExactRecoveryCapture(t), 0o600); err != nil {
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
	text := string(body)
	for _, want := range []string{
		"workqueue_execute_start: work struct 0xaaa: function 0x111",
		"workqueue_execute_end: work struct 0xaaa: function 0x222",
		"mm_filemap_add_to_page_cache: dev 12:48 ino 0x1234 pfn=77 ofs=4096",
		"f2fs_write_begin: dev=8:0 ino=0x9 pos=0 len=4096",
		"f2fs_write_end: dev=8:0 ino=0x9 pos=0 len=4096 copied=4096",
	} {
		if strings.Count(text, want) != 1 {
			t.Fatalf("trace_streamer exact source family count for %q != 1:\n%s", want, text)
		}
	}
	states := map[string]string{}
	for _, coverage := range result.TraceDBCoverage {
		if strings.HasPrefix(coverage.Family, "source_rawtrace_") {
			states[coverage.Family] = coverage.Metadata["publication_state"]
		}
	}
	for _, family := range traceDBRawExactRecoveryFamilies {
		if states["source_rawtrace_"+family] != "published_complete_exact_source_family" {
			t.Fatalf("exact source family %s coverage missing: %+v", family, result.TraceDBCoverage)
		}
	}
}
