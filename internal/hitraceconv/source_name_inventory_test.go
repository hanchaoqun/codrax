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

func TestTraceDBSourceNameInventoryAcceptsCommonFileTypeZeroAndFailsClosedPerTID(t *testing.T) {
	var capture bytes.Buffer
	writeFileHeader(&capture, 2)
	body := capture.Bytes()
	binary.LittleEndian.PutUint16(body[0:2], traceStreamerRawTraceMagic)
	body[2] = 0
	capture.Reset()
	capture.Write(body)
	writeSegment(&capture, segmentCmdlines, []byte(
		"100 app\n"+
			"200 worker\n"+
			"200 worker\n"+
			"300 first\n"+
			"300 second\n"+
			"400 unknown\n"+
			"bad row\n"))
	writeSegment(&capture, segmentRawTrace, make([]byte, 64))

	path := filepath.Join(t.TempDir(), "file-type-zero.sys")
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
	if !inventory.Coverage.Found || inventory.Coverage.RowsRead != 7 ||
		inventory.Coverage.RowsEmitted != 2 ||
		inventory.Names[100] != "app" || inventory.Names[200] != "worker" {
		t.Fatalf("source cmdline inventory mismatch: %+v", inventory)
	}
	if _, exists := inventory.Names[300]; exists {
		t.Fatalf("conflicting same-TID names were not withheld: %+v", inventory)
	}
	if _, exists := inventory.Names[400]; exists {
		t.Fatalf("placeholder name acquired display authority: %+v", inventory)
	}
	if inventory.Coverage.Metrics["cmdline_rows_duplicate_same_name"] != 1 ||
		inventory.Coverage.Metrics["cmdline_rows_conflicting_name"] != 1 ||
		inventory.Coverage.Metrics["cmdline_tids_ambiguous"] != 2 ||
		inventory.Coverage.Metrics["cmdline_rows_rejected_placeholder_name"] != 1 ||
		inventory.Coverage.Metrics["cmdline_rows_rejected_invalid_tid"] != 1 ||
		inventory.Coverage.Metadata["ambiguous_tid_witnesses"] != "300,400" ||
		inventory.Coverage.Metrics["source_envelope_official_rawtrace_v1"] != 1 {
		t.Fatalf("source cmdline audit metrics mismatch: %+v", inventory.Coverage)
	}
	raw := inventory.RawAuthority
	if !raw.Found || raw.RowsRead != 2 || raw.RowsEmitted != 2 ||
		raw.Metadata["inventory_state"] != "complete" ||
		raw.Metadata["event_format_state"] != "absent" ||
		raw.Metadata["raw_payload_state"] != "present_nonempty_unvalidated" ||
		raw.Metadata["decode_authority"] != "unavailable_event_format_segment_absent" ||
		raw.Metrics["raw_trace_segments"] != 1 || raw.Metrics["raw_trace_bytes"] != 64 {
		t.Fatalf("official raw authority inventory mismatch: %+v", raw)
	}
}

func TestTraceDBSourceNameInventoryKeepsOfficialAndLegacyEnvelopeAuthorityClosed(t *testing.T) {
	for _, test := range []struct {
		name       string
		magic      uint16
		wantFound  bool
		wantMetric string
	}{
		{name: "official rawtrace", magic: traceStreamerRawTraceMagic, wantFound: true, wantMetric: "source_envelope_official_rawtrace_v1"},
		{name: "legacy RMQ", magic: harmonyRMQMagic, wantFound: true, wantMetric: "source_envelope_legacy_rmq_v1"},
		{name: "near unknown", magic: traceStreamerRawTraceMagic - 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var capture bytes.Buffer
			writeFileHeader(&capture, 2)
			body := capture.Bytes()
			binary.LittleEndian.PutUint16(body[0:2], test.magic)
			capture.Reset()
			capture.Write(body)
			writeSegment(&capture, segmentCmdlines, []byte("123 exact-name\n"))

			path := filepath.Join(t.TempDir(), "source.sys")
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
			if inventory.Coverage.Found != test.wantFound {
				t.Fatalf("envelope admission=%t want %t: %+v", inventory.Coverage.Found, test.wantFound, inventory)
			}
			if !test.wantFound {
				if len(inventory.Names) != 0 ||
					!bytes.Contains([]byte(inventory.Coverage.Skipped), []byte("unsupported envelope magic")) {
					t.Fatalf("unknown envelope gained source-name authority: %+v", inventory)
				}
				return
			}
			if inventory.Names[123] != "exact-name" ||
				inventory.Coverage.Metrics[test.wantMetric] != 1 {
				t.Fatalf("proven envelope lost source name or provenance: %+v", inventory)
			}
			if !inventory.RawAuthority.Found ||
				inventory.RawAuthority.Metadata["raw_payload_state"] != "absent" ||
				inventory.RawAuthority.Metadata["decode_authority"] != "not_applicable_raw_payload_absent" {
				t.Fatalf("empty raw payload was not distinguished from missing authority: %+v", inventory.RawAuthority)
			}
		})
	}
}

func TestTraceDBSourceRawAuthorityDistinguishesEmptySupportedAndIncomplete(t *testing.T) {
	for _, test := range []struct {
		name          string
		build         func(*bytes.Buffer)
		wantInventory string
		wantFormat    string
		wantRaw       string
		wantDecoder   string
		wantEmitted   int
	}{
		{
			name: "present empty",
			build: func(capture *bytes.Buffer) {
				writeSegment(capture, segmentEventsFormat, nil)
				writeSegment(capture, segmentRawTrace, nil)
			},
			wantInventory: "complete",
			wantFormat:    "present_empty",
			wantRaw:       "present_empty",
			wantDecoder:   "unavailable_raw_payload_empty",
			wantEmitted:   2,
		},
		{
			name: "official payload decoder unavailable",
			build: func(capture *bytes.Buffer) {
				writeSegment(capture, segmentEventsFormat, []byte("name: sched_switch\nID: 1\n"))
				writeSegment(capture, segmentRawTrace, make([]byte, 4096))
			},
			wantInventory: "complete",
			wantFormat:    "present_nonempty_unvalidated",
			wantRaw:       "present_nonempty_unvalidated",
			wantDecoder:   "unavailable_official_page_decoder_not_implemented",
			wantEmitted:   2,
		},
		{
			name: "truncated payload",
			build: func(capture *bytes.Buffer) {
				var header [segmentHdrSize]byte
				binary.LittleEndian.PutUint32(header[0:4], segmentRawTrace)
				binary.LittleEndian.PutUint32(header[4:8], 4096)
				capture.Write(header[:])
				capture.WriteByte(0)
			},
			wantInventory: "incomplete",
			wantFormat:    "absent",
			wantRaw:       "absent",
			wantDecoder:   "unavailable_segment_inventory_incomplete",
			wantEmitted:   0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var capture bytes.Buffer
			writeFileHeader(&capture, 2)
			body := capture.Bytes()
			binary.LittleEndian.PutUint16(body[0:2], traceStreamerRawTraceMagic)
			body[2] = 0
			capture.Reset()
			capture.Write(body)
			test.build(&capture)

			path := filepath.Join(t.TempDir(), "raw-authority.sys")
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
			raw := inventory.RawAuthority
			if !raw.Found || raw.Metadata["inventory_state"] != test.wantInventory ||
				raw.Metadata["event_format_state"] != test.wantFormat ||
				raw.Metadata["raw_payload_state"] != test.wantRaw ||
				raw.Metadata["decode_authority"] != test.wantDecoder ||
				raw.RowsEmitted != test.wantEmitted {
				t.Fatalf("raw authority state mismatch: %+v", raw)
			}
			if test.wantInventory == "incomplete" &&
				(!bytes.Contains([]byte(raw.Skipped), []byte("segment_payload_exceeds_immutable_input")) ||
					raw.Metrics["segment_inventory_incomplete"] != 1) {
				t.Fatalf("incomplete inventory lost typed reason: %+v", raw)
			}
		})
	}
}

func TestTraceDBSourceRawProfileProbeReportsStrictCandidateWithoutPublishing(t *testing.T) {
	var capture bytes.Buffer
	writeFileHeader(&capture, 2)
	header := capture.Bytes()
	binary.LittleEndian.PutUint16(header[0:2], traceStreamerRawTraceMagic)
	header[2] = harmonyRMQFileType
	capture.Reset()
	capture.Write(header)
	format := strings.Join(syntheticFormatBlock("sched_switch", 90, []string{
		syntheticField("unsigned short", "common_type", 0, 2, false),
		syntheticField("unsigned char", "common_flags", 2, 1, false),
		syntheticField("unsigned char", "common_preempt_count", 3, 1, false),
		syntheticField("int", "common_pid", 4, 4, true),
	}), "\n")
	writeSegment(&capture, segmentEventsFormat, []byte(format))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{{
		EventID: 90, OffsetNS: 7, Content: make([]byte, 8),
	}}))
	writeSegment(&capture, 33, []byte("clock=boot\n"))

	path := filepath.Join(t.TempDir(), "official-profile.sys")
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
	probe := inventory.RawProfile
	if !probe.Found || probe.Role != "diagnostic_probe" ||
		probe.RowsRead != 1 || probe.RowsEmitted != 0 ||
		probe.Metadata["probe_state"] != "complete" ||
		probe.Metadata["event_format_probe_state"] != "parsed_strict" ||
		probe.Metadata["page_layout_state"] != "qword_length_cpu_candidate_all_pages" ||
		probe.Metadata["decoder_readiness"] != "structural_candidate_requires_fixture_parity" ||
		probe.Metadata["candidate_cpu_roster"] != "1" ||
		!strings.Contains(probe.Metadata["target_format_witnesses"], "sched_switch#90/fields=4/common_type_exact=true") ||
		!strings.Contains(probe.Metadata["unknown_segment_witnesses"], `type=33/bytes=11/`) ||
		!strings.Contains(probe.Metadata["unknown_segment_witnesses"], `/text="clock=boot\n"`) ||
		probe.Metrics["pages_probed"] != 1 ||
		probe.Metrics["pages_qword_length_cpu_candidate"] != 1 ||
		probe.Metrics["records_structurally_scanned"] != 1 ||
		probe.Metrics["records_matching_admitted_format"] != 1 ||
		probe.Metrics["candidate_records_sched_switch"] != 1 {
		t.Fatalf("official raw profile probe mismatch: %+v", probe)
	}
}

func TestTraceDBSourceRawProfileProbeRejectsCandidateLayoutWithoutAffectingAuthorityInventory(t *testing.T) {
	var capture bytes.Buffer
	writeFileHeader(&capture, 2)
	header := capture.Bytes()
	binary.LittleEndian.PutUint16(header[0:2], traceStreamerRawTraceMagic)
	capture.Reset()
	capture.Write(header)
	writeSegment(&capture, segmentEventsFormat, []byte(strings.Join(
		syntheticFormatBlock("sched_switch", 90, []string{
			syntheticField("unsigned short", "common_type", 0, 2, false),
		}), "\n")))
	page := make([]byte, tracePageSize)
	binary.LittleEndian.PutUint64(page[8:16], tracePageSize)
	page[16] = 1
	writeSegment(&capture, segmentRawTrace, page)

	path := filepath.Join(t.TempDir(), "different-page-layout.sys")
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
	probe := inventory.RawProfile
	if probe.Metadata["probe_state"] != "complete" ||
		probe.Metadata["page_layout_state"] != "candidate_rejected" ||
		probe.Metadata["decoder_readiness"] != "requires_different_page_layout" ||
		probe.Metrics["pages_probed"] != 1 ||
		probe.Metrics["pages_structurally_invalid"] != 1 ||
		probe.Metrics["page_probe_failure_logical_length_out_of_range"] != 1 ||
		probe.RowsEmitted != 0 {
		t.Fatalf("different official page layout did not fail candidate probe closed: %+v", probe)
	}
	raw := inventory.RawAuthority
	if raw.Metadata["raw_payload_state"] != "present_nonempty_unvalidated" ||
		raw.Metadata["decode_authority"] != "unavailable_official_page_decoder_not_implemented" {
		t.Fatalf("diagnostic probe changed source authority inventory: %+v", raw)
	}
}

func TestTraceDBSourceRawDecodeLedgerAdmitsStrictBlockedReasonWithoutPublishing(t *testing.T) {
	var capture bytes.Buffer
	writeFileHeader(&capture, 2)
	header := capture.Bytes()
	binary.LittleEndian.PutUint16(header[0:2], traceStreamerRawTraceMagic)
	header[2] = harmonyRMQFileType
	capture.Reset()
	capture.Write(header)
	format := strings.Join(syntheticFormatBlock("sched_blocked_reason", 32778, []string{
		syntheticField("unsigned short", "common_type", 0, 2, false),
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("int", "pid", 8, 4, true),
		syntheticField("void *", "caller", 12, 8, false),
		syntheticField("bool", "io_wait", 20, 1, false),
	}), "\n")
	content := make([]byte, 21)
	binary.LittleEndian.PutUint32(content[4:8], 77)
	binary.LittleEndian.PutUint32(content[8:12], 88)
	binary.LittleEndian.PutUint64(content[12:20], 0x1234)
	content[20] = 1
	writeSegment(&capture, segmentEventsFormat, []byte(format))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{{
		EventID: 32778, OffsetNS: 9, Content: content,
	}}))

	path := filepath.Join(t.TempDir(), "official-strict-decode.sys")
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
	if !decode.Found || decode.Role != "diagnostic_ledger" ||
		decode.Metadata["decode_state"] != "strict_target_ledger_complete" ||
		decode.Metadata["publication_authority"] != "withheld_rpd1_diagnostic_only" ||
		decode.RowsRead != 1 || decode.RowsEmitted != 0 ||
		decode.Metrics["records_with_admitted_format"] != 1 ||
		decode.Metrics["target_sched_blocked_reason_records"] != 1 ||
		decode.Metrics["target_sched_blocked_reason_envelope_admitted"] != 1 ||
		decode.Metrics["target_sched_blocked_reason_body_admitted"] != 1 ||
		!strings.Contains(decode.Metadata["format_record_witnesses"],
			"sched_blocked_reason#32778/records=1") ||
		!strings.Contains(decode.Metadata["target_formats_absent"], "trace_vsync") ||
		len(inventory.RawBlocked) != 1 ||
		inventory.RawBlocked[0].HeaderPID != 77 ||
		inventory.RawBlocked[0].TargetTID != 88 ||
		inventory.RawBlocked[0].IOWait != 1 ||
		inventory.RawBlocked[0].CallerRaw != 0x1234 ||
		inventory.RawBlocked[0].CPU != 1 {
		t.Fatalf("strict raw blocked-reason ledger mismatch: decode=%+v raw=%+v",
			decode, inventory.RawBlocked)
	}
}

func TestTraceDBSourceRawDecodeLedgerRetainsStrictSchedulerLiteRecordsWithoutPublishing(t *testing.T) {
	var capture bytes.Buffer
	writeFileHeader(&capture, 2)
	header := capture.Bytes()
	binary.LittleEndian.PutUint16(header[0:2], traceStreamerRawTraceMagic)
	header[2] = harmonyRMQFileType
	capture.Reset()
	capture.Write(header)
	format := strings.Join(append(
		syntheticFormatBlock("sched_switch_lite", 32772, []string{
			syntheticField("unsigned short", "common_type", 0, 2, false),
			syntheticField("int", "common_pid", 4, 4, true),
			syntheticField("int", "prev_pid", 8, 4, true),
			syntheticField("short", "prev_prio", 12, 2, true),
			syntheticField("unsigned long long", "prev_state", 16, 8, false),
			syntheticField("int", "next_pid", 24, 4, true),
			syntheticField("short", "next_prio", 28, 2, true),
			syntheticField("unsigned long long", "next_info", 32, 8, false),
		}),
		syntheticFormatBlock("sched_wakeup_lite", 32782, []string{
			syntheticField("unsigned short", "common_type", 0, 2, false),
			syntheticField("int", "common_pid", 4, 4, true),
			syntheticField("int", "pid", 8, 4, true),
			syntheticField("short", "prio", 12, 2, true),
			syntheticField("int", "target_cpu", 16, 4, true),
		})...,
	), "\n")
	switchContent := make([]byte, 40)
	binary.LittleEndian.PutUint16(switchContent[0:2], 32772)
	switchContent[2], switchContent[3] = 1, 2
	binary.LittleEndian.PutUint32(switchContent[4:8], 77)
	binary.LittleEndian.PutUint32(switchContent[8:12], 88)
	binary.LittleEndian.PutUint16(switchContent[12:14], 0xfffe)
	binary.LittleEndian.PutUint64(switchContent[16:24], 0x100)
	binary.LittleEndian.PutUint32(switchContent[24:28], 99)
	binary.LittleEndian.PutUint16(switchContent[28:30], 53)
	nextInfo := uint64(0x3fff) | uint64(50)<<32 | uint64(3)<<42 | uint64(1)<<44 |
		uint64(2)<<45 | uint64(14)<<48 | uint64(1)<<60
	binary.LittleEndian.PutUint64(switchContent[32:40], nextInfo)
	wakeupContent := make([]byte, 20)
	binary.LittleEndian.PutUint16(wakeupContent[0:2], 32782)
	wakeupContent[2], wakeupContent[3] = 4, 5
	binary.LittleEndian.PutUint32(wakeupContent[4:8], 77)
	binary.LittleEndian.PutUint32(wakeupContent[8:12], 99)
	binary.LittleEndian.PutUint16(wakeupContent[12:14], 53)
	binary.LittleEndian.PutUint32(wakeupContent[16:20], 3)
	writeSegment(&capture, segmentEventsFormat, []byte(format))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 32772, OffsetNS: 9, Content: switchContent},
		{EventID: 32782, OffsetNS: 10, Content: wakeupContent},
	}))

	path := filepath.Join(t.TempDir(), "official-scheduler-lite-decode.sys")
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
		decode.RowsRead != 2 || decode.RowsEmitted != 0 ||
		decode.Metrics["target_sched_switch_lite_body_admitted"] != 1 ||
		decode.Metrics["target_sched_wakeup_lite_body_admitted"] != 1 ||
		decode.Metrics["target_sched_switch_lite_next_info_unknown_tail_bits"] != 1 ||
		!strings.Contains(decode.Metadata["target_format_geometry_witnesses"],
			"sched_switch_lite#32772[") ||
		!strings.Contains(decode.Metadata["target_format_geometry_witnesses"],
			"sched_wakeup_lite#32782[") ||
		!strings.Contains(decode.Metadata["scheduler_lite_format_geometry_witnesses"],
			"prev_prio@12:2:signed=true:type=short") ||
		!strings.Contains(decode.Metadata["scheduler_lite_format_geometry_witnesses"],
			"target_cpu@16:4:signed=true:type=int") ||
		len(inventory.RawSwitchLite) != 1 || len(inventory.RawWakeupLite) != 1 {
		t.Fatalf("strict scheduler-lite ledger mismatch: decode=%+v switch=%+v wakeup=%+v",
			decode, inventory.RawSwitchLite, inventory.RawWakeupLite)
	}
	gotSwitch := inventory.RawSwitchLite[0]
	if gotSwitch.HeaderPID != 77 || gotSwitch.Flags != 1 || gotSwitch.PreemptCount != 2 ||
		gotSwitch.CPU != 1 || gotSwitch.PrevTID != 88 || gotSwitch.PrevPriority != -2 ||
		gotSwitch.PrevState != 0x100 || gotSwitch.NextTID != 99 ||
		gotSwitch.NextPriority != 53 || gotSwitch.NextInfo != nextInfo {
		t.Fatalf("retained sched_switch_lite mismatch: %+v", gotSwitch)
	}
	gotWakeup := inventory.RawWakeupLite[0]
	if gotWakeup.HeaderPID != 77 || gotWakeup.Flags != 4 || gotWakeup.PreemptCount != 5 ||
		gotWakeup.CPU != 1 || gotWakeup.TargetTID != 99 ||
		gotWakeup.Priority != 53 || gotWakeup.TargetCPU != 3 {
		t.Fatalf("retained sched_wakeup_lite mismatch: %+v", gotWakeup)
	}
}

func TestTraceDBSourceNameInventoryNarrowsNamespaceDuplicatesToHostSchedulerLane(t *testing.T) {
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 10}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 100, IPID: 1, SwitchCount: 2}
	index.ByITID[2] = traceDBThread{ITID: 2, TID: 200, IPID: 1}
	index.ByITID[3] = traceDBThread{ITID: 3, TID: 200, IPID: 1}
	index.ByITID[4] = traceDBThread{ITID: 4, TID: 300, IPID: 1, SwitchCount: 3}
	index.ByITID[5] = traceDBThread{ITID: 5, TID: 300, IPID: 1}
	index.ByITID[6] = traceDBThread{ITID: 6, TID: 400, IPID: 1, Name: "db-name", SwitchCount: 1}
	index.SourceDisplayNameByTID = map[int64]string{
		100: "unique",
		200: "ambiguous-namespace",
		300: "host-scheduler",
		400: "source-name",
	}
	buildTraceDBThreadSecondaryIndexes(&index)

	if name, source := traceDBThreadDisplayName(index, index.ByITID[1]); name != "unique" ||
		source != traceDBDisplayNameSourceCmdline {
		t.Fatalf("unique source name not recovered: name=%q source=%q", name, source)
	}
	for _, itid := range []int64{2, 3, 5} {
		if name, source := traceDBThreadDisplayName(index, index.ByITID[itid]); name != "" || source != "" {
			t.Fatalf("namespace-ambiguous/non-host candidate gained a name: itid=%d name=%q source=%q", itid, name, source)
		}
	}
	if name, source := traceDBThreadDisplayName(index, index.ByITID[4]); name != "host-scheduler" ||
		source != traceDBDisplayNameSourceCmdline {
		t.Fatalf("sole scheduler-active host candidate not recovered: name=%q source=%q", name, source)
	}
	if name, source := traceDBThreadDisplayName(index, index.ByITID[6]); name != "db-name" ||
		source != traceDBDisplayNameDirect {
		t.Fatalf("canonical DB display name lost precedence: name=%q source=%q", name, source)
	}
}
