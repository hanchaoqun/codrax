package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
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
