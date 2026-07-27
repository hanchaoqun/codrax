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
