package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func traceDBRawWakeupNameTestAuthority() traceDBSchedulerAuthority {
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 68, Name: "system"}
	index.ByITID[398] = traceDBThread{
		ITID: 398, TID: 29352, IPID: 1, SwitchCount: 816,
	}
	buildTraceDBThreadSecondaryIndexes(&index)
	return traceDBTestCompleteSchedulerAuthority(index)
}

func traceDBRawWakeupNameTestInventory(
	rows []traceDBRawSchedWakeupNewNameRecord,
) *traceDBSourceNameInventory {
	return &traceDBSourceNameInventory{
		RawDecode: TraceDBCoverage{
			Found: true,
			Role:  "diagnostic_ledger",
			Metadata: map[string]string{
				"decode_state": "strict_target_ledger_complete",
			},
			Metrics: map[string]int64{
				"target_sched_wakeup_new_name_records_retained": int64(len(rows)),
			},
		},
		RawWakeupNames: append([]traceDBRawSchedWakeupNewNameRecord(nil), rows...),
	}
}

func TestTraceDBRawWakeupNewNameRecoversOnlyUnresolvedExactIncarnation(t *testing.T) {
	authority := traceDBRawWakeupNameTestAuthority()
	inventory := traceDBRawWakeupNameTestInventory(
		[]traceDBRawSchedWakeupNewNameRecord{
			{TimestampNS: 100, TargetTID: 29352, Name: "FrameWorker"},
			{TimestampNS: 200, TargetTID: 29352, Name: "FrameWorker"},
		})
	coverage := traceDBApplyRawWakeupNewDisplayNames(inventory, &authority)
	thread := authority.identities.ByITID[398]
	name, source := traceDBThreadDisplayName(authority.identities, thread)
	if name != "FrameWorker" || source != traceDBDisplayNameSourceRawWakeupNew ||
		coverage.RowsRead != 2 || coverage.RowsEmitted != 1 ||
		coverage.Metrics["raw_name_rows_identity_admitted"] != 2 ||
		coverage.Metrics["thread_names_recovered_source_wakeup_new"] != 1 ||
		coverage.Metadata["recovery_state"] != "published_display_names" {
		t.Fatalf("exact raw wakeup name was not recovered: name=%q source=%q coverage=%+v",
			name, source, coverage)
	}
}

func TestTraceDBRawWakeupNewNameConflictsAndExistingNamesFailClosed(t *testing.T) {
	t.Run("conflicting raw names", func(t *testing.T) {
		authority := traceDBRawWakeupNameTestAuthority()
		coverage := traceDBApplyRawWakeupNewDisplayNames(
			traceDBRawWakeupNameTestInventory(
				[]traceDBRawSchedWakeupNewNameRecord{
					{TimestampNS: 100, TargetTID: 29352, Name: "old"},
					{TimestampNS: 200, TargetTID: 29352, Name: "new"},
				}), &authority)
		if coverage.RowsEmitted != 0 ||
			coverage.Metrics["canonical_itids_with_name_conflict"] != 1 ||
			authority.threadDisplayName(authority.identities.ByITID[398]) != "" {
			t.Fatalf("conflicting names changed the display identity: %+v", coverage)
		}
	})

	t.Run("existing canonical display", func(t *testing.T) {
		authority := traceDBRawWakeupNameTestAuthority()
		authority.identities.DisplayNameByITID[398] = "canonical"
		authority.identities.DisplayNameSourceByITID[398] = traceDBDisplayNameDirect
		coverage := traceDBApplyRawWakeupNewDisplayNames(
			traceDBRawWakeupNameTestInventory(
				[]traceDBRawSchedWakeupNewNameRecord{{
					TimestampNS: 100, TargetTID: 29352, Name: "raw-name",
				}}), &authority)
		if coverage.RowsEmitted != 0 ||
			coverage.Metrics["raw_name_rows_existing_different"] != 1 ||
			authority.threadDisplayName(authority.identities.ByITID[398]) != "canonical" {
			t.Fatalf("raw name overrode an existing display name: %+v", coverage)
		}
	})
}

func TestTraceDBSourceRawDecodeRetainsWakeupNewDisplayName(t *testing.T) {
	var capture bytes.Buffer
	writeFileHeader(&capture, 2)
	header := capture.Bytes()
	binary.LittleEndian.PutUint16(header[0:2], traceStreamerRawTraceMagic)
	header[2] = harmonyRMQFileType
	capture.Reset()
	capture.Write(header)

	format := strings.Join(syntheticFormatBlock("sched_wakeup_new", 32781, []string{
		syntheticField("unsigned short", "common_type", 0, 2, false),
		syntheticField("unsigned char", "common_flags", 2, 1, false),
		syntheticField("unsigned char", "common_preempt_count", 3, 1, false),
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("char", "pname[16]", 8, 16, true),
		syntheticField("int", "pid", 24, 4, true),
		syntheticField("s16", "prio", 28, 2, true),
		syntheticField("s8", "target_cpu", 30, 1, true),
	}), "\n")
	content := make([]byte, 31)
	binary.LittleEndian.PutUint16(content[0:2], 32781)
	content[2], content[3] = 1, 2
	binary.LittleEndian.PutUint32(content[4:8], 77)
	copy(content[8:24], []byte("FrameWorker\x00"))
	binary.LittleEndian.PutUint32(content[24:28], 29352)
	binary.LittleEndian.PutUint16(content[28:30], 53)
	content[30] = 7
	writeSegment(&capture, segmentEventsFormat, []byte(format))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents(
		[]syntheticRawEvent{{EventID: 32781, OffsetNS: 9, Content: content}}))

	path := filepath.Join(t.TempDir(), "official-wakeup-name.sys")
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
	if inventory.RawDecode.Metadata["decode_state"] != "strict_target_ledger_complete" ||
		inventory.RawDecode.Metrics["target_sched_wakeup_new_body_admitted"] != 1 ||
		inventory.RawDecode.Metrics["target_sched_wakeup_new_name_records_retained"] != 1 ||
		len(inventory.RawWakeupNames) != 1 ||
		inventory.RawWakeupNames[0].TargetTID != 29352 ||
		inventory.RawWakeupNames[0].Name != "FrameWorker" {
		t.Fatalf("strict raw wakeup-new name retention mismatch: decode=%+v names=%+v",
			inventory.RawDecode, inventory.RawWakeupNames)
	}
}

func TestTraceDBRawWakeupNewNameIsWiredIntoSchedulerExport(t *testing.T) {
	path := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 68, '')",
		"INSERT INTO process VALUES (2, 200, 'Creator')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (398, 29352, 1, '', 0, 0, 1)",
		"INSERT INTO thread VALUES (2, 200, 2, 'creator', 0, 1, 1)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
		"INSERT INTO sched_slice VALUES (120, NULL, 7, 'R', 53, 398)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT)",
		"INSERT INTO instant VALUES (100, 'sched_wakeup_new', 398, 2, 'itid')",
		"CREATE TABLE raw (id INT, ts INT, name TEXT, cpu INT, itid INT)",
		"INSERT INTO raw VALUES (1, 100, 'sched_wakeup', 7, 398)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (2, 90, 20, 2, 'Running')",
		"CREATE TABLE callstack (ts INT, itid INT, callid INT)",
		"CREATE TABLE syscall (ts INT, itid INT)",
		"CREATE TABLE native_hook (start_ts INT, itid INT)",
		"CREATE TABLE frame_slice (ts INT, itid INT)",
	})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	tdb.sourceNameInventory = traceDBRawWakeupNameTestInventory(
		[]traceDBRawSchedWakeupNewNameRecord{{
			TimestampNS: 100, TargetTID: 29352, Name: "FrameWorker",
		}})
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	syncSpans := newTraceDBTestSyncSpanAuthority(t)
	coverage, _, err := exportTraceDBSchedulerFamilies(
		context.Background(), tdb, sink, syncSpans)
	if err != nil {
		t.Fatal(err)
	}
	coverage, _, _ = finalizeTraceDBTestSyncSpans(t, sink, syncSpans, coverage)
	outPath := filepath.Join(t.TempDir(), "wakeup-name.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := sink.prepareAndWriteForTest(context.Background(), out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body),
		"sched_wakeup_new: comm=FrameWorker pid=29352") {
		t.Fatalf("production scheduler export did not consume the recovered display name:\n%s", body)
	}
	var nameCoverage, threadCoverage *TraceDBCoverage
	for index := range coverage {
		item := &coverage[index]
		switch {
		case item.Family == "resolver.source_raw_wakeup_name":
			nameCoverage = item
		case item.Family == "resolver" && item.Table == "thread":
			threadCoverage = item
		}
	}
	if nameCoverage == nil || nameCoverage.RowsEmitted != 1 ||
		threadCoverage == nil ||
		threadCoverage.Metrics["thread_names_recovered_source_wakeup_new"] != 1 ||
		threadCoverage.Metrics["unresolved_thread_names"] != 0 {
		t.Fatalf("production name-recovery coverage drifted: name=%+v thread=%+v",
			nameCoverage, threadCoverage)
	}
}
