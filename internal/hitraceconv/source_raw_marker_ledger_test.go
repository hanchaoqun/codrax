package hitraceconv

import (
	"encoding/binary"
	"testing"
)

func traceDBRawMarkerLegacyEvent(start byte, pid int32, name string) (eventFormat, []byte) {
	format := eventFormat{
		ID: 32925, Name: "tracing_mark_write",
		Fields: []eventField{
			{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
			{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
			{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
			{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "int", Name: "start", Offset: 8, Size: 4, Signed: true},
			{Type: "int", Name: "pid", Offset: 12, Size: 4, Signed: true},
			{Type: "char", Name: "name[64]", Offset: 16, Size: 64},
		},
	}
	content := make([]byte, 80)
	binary.LittleEndian.PutUint16(content[0:2], uint16(format.ID))
	content[2], content[3] = 1, 2
	binary.LittleEndian.PutUint32(content[4:8], 77)
	binary.LittleEndian.PutUint32(content[8:12], uint32(start))
	binary.LittleEndian.PutUint32(content[12:16], uint32(pid))
	copy(content[16:80], name)
	return format, content
}

func TestTraceDBRawMarkerLedgerRetainsExactNamespaceBAndE(t *testing.T) {
	acc := newTraceDBSourceRawDecodeAccumulator()
	beginFormat, begin := traceDBRawMarkerLegacyEvent(1, 1234, "render")
	endFormat, end := traceDBRawMarkerLegacyEvent(0, 1234, "")
	acc.observeRecord(beginFormat, begin, 3, 1_000_000)
	acc.observeRecord(endFormat, end, 4, 4_000_000)

	if len(acc.markerRecords) != 2 {
		t.Fatalf("marker records=%+v", acc.markerRecords)
	}
	first, second := acc.markerRecords[0], acc.markerRecords[1]
	if !first.Admitted || first.Action != "B" || first.PayloadPID != 1234 ||
		first.HeaderPID != 77 || first.Name != "render" || first.Buffer != "B|1234|render" ||
		first.CPU != 3 || first.Flags != 1 || first.PreemptCount != 2 ||
		!second.Admitted || second.Action != "E" || second.PayloadPID != 1234 ||
		second.Buffer != "E|1234|" || second.CPU != 4 {
		t.Fatalf("exact marker retention mismatch: %+v", acc.markerRecords)
	}
	if acc.coverage.Metrics["target_marker_sync_records_retained"] != 2 ||
		acc.coverage.Metrics["target_marker_sync_endpoints_admitted"] != 2 ||
		acc.coverage.Metrics["target_marker_endpoint_b_admitted"] != 1 ||
		acc.coverage.Metrics["target_marker_endpoint_e_admitted"] != 1 {
		t.Fatalf("marker metrics=%+v", acc.coverage.Metrics)
	}
}

func TestTraceDBRawMarkerLedgerRetainsCarrierFailureAsLanePoison(t *testing.T) {
	acc := newTraceDBSourceRawDecodeAccumulator()
	format, content := traceDBRawMarkerLegacyEvent(1, 1234, "render")
	format.Fields[len(format.Fields)-1].Name = "name[63]"
	acc.observeRecord(format, content, 3, 1_000_000)

	if len(acc.markerRecords) != 1 || acc.markerRecords[0].Admitted ||
		acc.markerRecords[0].HeaderPID != 77 ||
		acc.markerRecords[0].RejectReason == "" {
		t.Fatalf("carrier rejection was not localized: %+v", acc.markerRecords)
	}
	if acc.coverage.Metrics["target_marker_carrier_rejections_retained"] != 1 {
		t.Fatalf("marker metrics=%+v", acc.coverage.Metrics)
	}
}

func TestTraceDBRawMarkerLedgerCoversPrintAndCanonicalSchemaRejection(t *testing.T) {
	acc := newTraceDBSourceRawDecodeAccumulator()
	printBegin := directMarkerDataLocFixture("print", []byte("B|4321|frame"), true)
	invalidEnd := directMarkerDataLocFixture(
		"tracing_mark_write", []byte("E|4321|D0005"), false)
	acc.observeRecord(printBegin.format, printBegin.content, 5, 1_000_000)
	acc.observeRecord(invalidEnd.format, invalidEnd.content, 6, 2_000_000)

	if len(acc.markerRecords) != 2 {
		t.Fatalf("marker records=%+v", acc.markerRecords)
	}
	begin, end := acc.markerRecords[0], acc.markerRecords[1]
	if !begin.Admitted || begin.Action != "B" || begin.PayloadPID != 4321 ||
		begin.Name != "frame" || begin.HeaderPID != 100 ||
		end.Admitted || end.Action != "E" ||
		end.RejectReason != "invalid_end_tag" || end.Buffer != "E|4321|D0005" {
		t.Fatalf("cross-carrier/schema verdict mismatch: %+v", acc.markerRecords)
	}
	if acc.coverage.Metrics["target_marker_carrier_records"] != 2 ||
		acc.coverage.Metrics["target_marker_sync_records_retained"] != 2 ||
		acc.coverage.Metrics["target_marker_sync_endpoints_admitted"] != 1 ||
		acc.coverage.Metrics["target_marker_sync_endpoints_rejected"] != 1 ||
		acc.coverage.Metrics["target_marker_endpoint_e_rejected"] != 1 {
		t.Fatalf("marker metrics=%+v", acc.coverage.Metrics)
	}
}

func TestTraceDBRawMarkerLedgerRetainsMalformedAsyncAsSyncStackPoison(t *testing.T) {
	acc := newTraceDBSourceRawDecodeAccumulator()
	malformed := directMarkerDataLocFixture(
		"tracing_mark_write", []byte("S|42||7"), false)
	acc.observeRecord(malformed.format, malformed.content, 2, 1_000_000)

	if len(acc.markerRecords) != 1 ||
		acc.markerRecords[0].Action != "S" ||
		acc.markerRecords[0].Admitted ||
		acc.markerRecords[0].RejectReason != "schema_empty_name" ||
		acc.coverage.Metrics["target_marker_sync_poison_records_retained"] != 1 ||
		acc.coverage.Metrics["target_marker_endpoint_s_rejected"] != 1 {
		t.Fatalf("malformed async endpoint did not poison the emitter sync stack: records=%+v metrics=%+v",
			acc.markerRecords, acc.coverage.Metrics)
	}
}

func TestTraceDBRawMarkerLedgerPublishesExactEndpointActionCensusWithoutRetainingCleanAsync(t *testing.T) {
	acc := newTraceDBSourceRawDecodeAccumulator()
	fixtures := []struct {
		payload string
		metric  string
	}{
		{"S|42|async|7", "target_marker_endpoint_s_admitted"},
		{"F|42|async|7", "target_marker_endpoint_f_admitted"},
		{"G|42|track|async|7", "target_marker_endpoint_g_admitted"},
		{"H|42|track|7", "target_marker_endpoint_h_admitted"},
		{"N|42|track|instant", "target_marker_endpoint_n_admitted"},
		{"I|42|instant", "target_marker_endpoint_i_admitted"},
	}
	for index, fixture := range fixtures {
		row := directMarkerDataLocFixture("print", []byte(fixture.payload), true)
		acc.observeRecord(row.format, row.content, index, uint64(index+1)*1_000_000)
		if acc.coverage.Metrics[fixture.metric] != 1 {
			t.Fatalf("payload=%q metric=%s metrics=%+v",
				fixture.payload, fixture.metric, acc.coverage.Metrics)
		}
	}
	counter := directMarkerDataLocFixture("print", []byte("C|42|counter|1"), true)
	acc.observeRecord(counter.format, counter.content, 7, 8_000_000)
	if acc.coverage.Metrics["target_marker_non_endpoint_payloads"] != 1 ||
		acc.coverage.Metrics["target_marker_non_sync_payloads"] != int64(len(fixtures)+1) ||
		len(acc.markerRecords) != 0 {
		t.Fatalf("action census changed publication ledger: records=%+v metrics=%+v",
			acc.markerRecords, acc.coverage.Metrics)
	}
}
