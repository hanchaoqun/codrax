package tracequery

import (
	"strings"
	"testing"
	"unsafe"
)

func TestOfficialEBPFIntervalWireRoundTrip(t *testing.T) {
	details := `{"return_value":"4","return_value_known":true,"error_code":"","error_code_known":false,"fd":3,"fd_known":true,"file_id":9,"file_id_known":true,"size_bytes":4096,"size_known":true,"arguments":["a","b","",""],"argument_known_mask":3}`
	interval := OfficialEBPFInterval{
		Family: EBPFFamilyFilesystem, TimestampNS: 100, EndTimestampNS: 150, DurationNS: 50,
		SourceRow: 0, TypeID: 2, InternalProcessID: 1, InternalThreadID: 2,
		PID: 100, TID: 200, IdentityStatus: "resolved",
		CallchainID: 5, CallchainStatus: "available", DetailsJSON: details,
	}
	line, err := FormatOfficialEBPFInterval(interval)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(line, "[000]") || !strings.HasPrefix(line, officialEBPFIntervalPrefix+" ") {
		t.Fatalf("eBPF relation fabricated a physical envelope: %q", line)
	}
	if ts, ok := ParseLineTimestampNS(line); !ok || ts != 100 {
		t.Fatalf("eBPF timestamp=(%d,%t), want (100,true)", ts, ok)
	}
	event, ok := ParseLine(7, line, newStringInterner())
	if !ok || event.Type != EventEBPFInterval || event.CPU != -1 ||
		event.PID != 200 || event.TGID != 100 || event.PluginFields == nil ||
		event.PluginFields.EBPFInterval == nil ||
		event.PluginFields.EBPFInterval.EndTimestampNS != 150 ||
		event.PluginFields.EBPFInterval.CallchainStatus != "available" ||
		event.PluginFields.EBPFInterval.DetailsJSON != details {
		t.Fatalf("eBPF typed relation drifted: %+v", event)
	}
	wantBytes := int64(unsafe.Sizeof(PluginFields{})) + int64(unsafe.Sizeof(EBPFIntervalFields{}))
	if got := eventSideTableBytes(&event); got != wantBytes {
		t.Fatalf("eBPF relation side-table bytes=%d, want %d", got, wantBytes)
	}
	stats := ComputeWindowStats(&Index{Events: []Event{event}}, Query{})
	if stats.FilesystemEventCount != 0 || stats.MemoryEventCount != 0 || stats.StorageEventCount != 0 {
		t.Fatalf("typed eBPF interval leaked into unaudited legacy resource aggregates: %+v", stats)
	}
}

func TestOfficialEBPFIntervalClosedWire(t *testing.T) {
	details := `{"size_bytes":0,"size_known":false,"address":"","address_known":false}`
	base, err := FormatOfficialEBPFInterval(OfficialEBPFInterval{
		Family: EBPFFamilyPagedMemory, TimestampNS: 10, EndTimestampNS: 10, DurationNS: 0,
		IdentityStatus: "unavailable", CallchainID: -1, CallchainStatus: "absent",
		DetailsJSON: details,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		strings.Replace(base, "ts_ns=10", "ts_ns=010", 1),
		strings.Replace(base, "payload_b64=", "payload_b64=AA", 1),
		base + " extra=x",
		strings.Replace(base, "/v1", "/v2", 1),
	} {
		if _, ok := ParseLine(1, line, newStringInterner()); ok {
			t.Fatalf("accepted non-canonical eBPF wire: %q", line)
		}
	}
	bad := OfficialEBPFInterval{
		Family: EBPFFamilyPagedMemory, TimestampNS: 10, EndTimestampNS: 11, DurationNS: 0,
		IdentityStatus: "unavailable", CallchainID: -1, CallchainStatus: "absent",
		DetailsJSON: details,
	}
	if _, err := FormatOfficialEBPFInterval(bad); err == nil {
		t.Fatal("accepted duration/end mismatch")
	}
}
