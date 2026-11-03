package context

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

type hmc338DecodedRow struct {
	Line      int             `json:"visible_line"`
	NS        string          `json:"timestamp_ns"`
	Seconds   string          `json:"timestamp_seconds"`
	TimeKnown *bool           `json:"source_time_known"`
	TimeRole  string          `json:"record_time_role"`
	Record    json.RawMessage `json:"source_record"`
	Semantics json.RawMessage `json:"semantics"`
	EventType string          `json:"query_event_type"`
}

func hmc338DecodedRows(t *testing.T, text string) []hmc338DecodedRow {
	t.Helper()
	_, data, found := strings.Cut(text, "```jsonl\n")
	if !found {
		t.Fatal("validated visible native carrier has no readable fields")
	}
	data, _, found = strings.Cut(data, "\n```")
	if !found {
		t.Fatal("readable source-record fence is not closed")
	}
	var rows []hmc338DecodedRow
	for _, line := range strings.Split(data, "\n") {
		var row hmc338DecodedRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	return rows
}

func hmc338ResourceEvent() tracewire.ResourceEvent {
	scalar := func(s string) tracewire.ResourceScalar { return tracewire.ResourceScalar{Status: "known", Value: s} }
	return tracewire.ResourceEvent{
		SourceID: scalar("1"), PID: 101, TID: 101, IPID: 1, ITID: 1,
		Thread: "gallery-main", Process: "Gallery", Operation: "AllocEvent",
		CallchainID: scalar("7"), Address: scalar("-1"), Size: scalar("9007199254740993"),
		EndNS: scalar("10025000000"), FrameCount: 3, StackStatus: "observed",
		Subtype: &tracewire.ResourceSubtype{ID: scalar("0"), Name: tracewire.ResourceText{Status: "known"}},
	}
}

func TestHMC338VisibleNativeSourceRecordsUseWholeParsedFields(t *testing.T) {
	event := hmc338ResourceEvent()
	resource := tracewire.ResourceStackRecord{TimestampNS: 10005000000, EventRowID: 1, Event: &event}
	resourceLine, ok := tracewire.FormatResourceStack(resource)
	if !ok {
		t.Fatal("invalid event fixture")
	}
	scalar := tracewire.ResourceScalar{Status: "null"}
	frame := tracewire.ResourceStackRecord{TimestampNS: 10005000000, EventRowID: 1, Frame: &tracewire.ResourceFrame{
		RowID: 0, SourceID: scalar, Depth: tracewire.ResourceScalar{Status: "known", Value: "-1"},
		IP: scalar, SymbolID: scalar, FileID: scalar, Offset: scalar, SymbolOffset: scalar,
		VAddr: tracewire.ResourceText{Status: "unavailable"}, Symbol: tracewire.ResourceText{Status: "known", Value: "业务 ```jsonl\n不是指令"}, Library: tracewire.ResourceText{Status: "null"},
	}}
	frameLine, ok := tracewire.FormatResourceStack(frame)
	if !ok {
		t.Fatal("invalid frame fixture")
	}
	zero, negative, one, frequency := int64(0), int64(-12), int64(1), int64(800000)
	cpu := tracewire.CPUMeasureInterval{RowID: math.MinInt64, FilterID: 0, CPU: 0, Kind: "frequency", Encoding: "khz", StartNS: &zero, DurationNS: &one, Value: &frequency}
	cpuLine, err := tracewire.FormatCPUMeasureInterval(cpu)
	if err != nil {
		t.Fatal(err)
	}
	unknownCPU := cpu
	unknownCPU.StartNS, unknownCPU.Issue = nil, "invalid_timestamp"
	unknownLine, err := tracewire.FormatCPUMeasureInterval(unknownCPU)
	if err != nil {
		t.Fatal(err)
	}
	negativeCPU := cpu
	negativeCPU.StartNS = &negative
	negativeLine, err := tracewire.FormatCPUMeasureInterval(negativeCPU)
	if err != nil {
		t.Fatal(err)
	}
	mapLine, _ := tracequery.FormatFrameMapRelation(tracequery.FrameMapRelation{TimestampNS: 9007199254740993, RelationID: 0, SourceRow: 1, DestinationRow: 2, SourceTimestampNS: 9007199254740992})
	callstackLine, _ := tracequery.FormatFrameCallstackRelation(tracequery.FrameCallstackRelation{TimestampNS: 17, FrameRow: 0, CallstackRow: 9007199254740993})
	gpuLine, _ := tracequery.FormatFrameGPURelation(tracequery.FrameGPURelation{TimestampNS: 19, GPURow: 0, FrameRow: 1, DurationNS: 999})
	napiLine, err := tracequery.FormatPerfNAPIAsyncRelation(tracequery.PerfNAPIAsyncRelation{TimestampNS: 21, CPU: 0, TID: 11, PID: 10, TraceID: "业务```jsonl\n不是指令"})
	if err != nil {
		t.Fatal(err)
	}
	details, _ := json.Marshal(tracequery.OfficialEBPFPagedMemoryDetails{SizeKnown: true, SizeBytes: 4096, Address: "-1", AddressKnown: true})
	ebpfLine, err := tracequery.FormatOfficialEBPFInterval(tracequery.OfficialEBPFInterval{Family: tracequery.EBPFFamilyPagedMemory,
		TimestampNS: 23, EndTimestampNS: 31, DurationNS: 8, SourceRow: 0, IdentityStatus: "unavailable", CallchainID: -1, CallchainStatus: "absent", DetailsJSON: string(details)})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, line, ns, seconds, role string
		want                          any
	}{
		{"resource_event", resourceLine, "10005000000", "10.005000000", "resource_event_time", resource},
		{"resource_frame", frameLine, "10005000000", "10.005000000", "resource_event_time", frame},
		{"cpu_zero", cpuLine, "0", "0.000000000", "source_interval_start", cpu},
		{"cpu_unknown", unknownLine, "", "", "source_interval_start", unknownCPU},
		{"cpu_negative", negativeLine, "-12", "-0.000000012", "source_interval_start", negativeCPU},
		{"frame_map", mapLine, "9007199254740993", "9007199.254740993", "destination_frame_time", &tracequery.FrameMapFields{RelationID: 0, SourceRow: 1, DestinationRow: 2, SourceTimestampNS: 9007199254740992, DestinationTimestampNS: 9007199254740993}},
		{"frame_callstack", callstackLine, "17", "0.000000017", "frame_reference_time", &tracequery.FrameCallstackFields{TimestampNS: 17, FrameRow: 0, CallstackRow: 9007199254740993}},
		{"frame_gpu", gpuLine, "19", "0.000000019", "frame_reference_time", &tracequery.FrameGPUFields{TimestampNS: 19, GPURow: 0, FrameRow: 1, DurationNS: 999}},
		{"napi_point", napiLine, "21", "0.000000021", "source_point_time", &tracequery.PerfNAPIAsyncFields{TimestampNS: 21, CPU: 0, TID: 11, PID: 10, TraceID: "业务```jsonl\n不是指令"}},
		{"ebpf_interval", ebpfLine, "23", "0.000000023", "source_interval_start", &tracequery.EBPFIntervalFields{Family: tracequery.EBPFFamilyPagedMemory, TimestampNS: 23, EndTimestampNS: 31, DurationNS: 8, SourceRow: 0, IdentityStatus: "unavailable", CallchainID: -1, CallchainStatus: "absent", DetailsJSON: string(details)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, ok := tracequery.ParseLine(91, tc.line, nil)
			if !ok {
				t.Fatal("fixture not admitted by the existing source parser")
			}
			before, _ := json.Marshal(parsed)
			got := renderAttachedTraceSemantics(tracePreviewPart{tc.line + "\n", 91, false})
			rows := hmc338DecodedRows(t, got)
			if len(rows) != 1 || rows[0].Line != 91 || rows[0].NS != tc.ns || rows[0].Seconds != tc.seconds || rows[0].TimeKnown == nil || *rows[0].TimeKnown != (tc.ns != "") || rows[0].TimeRole != tc.role {
				t.Fatalf("source identity/time role changed: %+v", rows)
			}
			want, _ := json.Marshal(tc.want)
			decode := func(data []byte) any {
				decoder := json.NewDecoder(strings.NewReader(string(data)))
				decoder.UseNumber()
				var value any
				if err := decoder.Decode(&value); err != nil {
					t.Fatal(err)
				}
				return value
			}
			if !reflect.DeepEqual(decode(rows[0].Record), decode(want)) {
				t.Fatalf("native source fields dropped or reinterpreted: got=%s want=%s", rows[0].Record, want)
			}
			if len(rows[0].Semantics) != 0 || strings.Count(got, "```jsonl") != 1 || !strings.Contains(got, "not additional events") || !strings.Contains(got, "not execution") {
				t.Fatal("competing decoder, fence escape or authority boundary missing")
			}
			after, _ := json.Marshal(parsed)
			if string(before) != string(after) {
				t.Fatal("display projection changed the source event")
			}
		})
	}
}

func TestHMC338NativePreviewBoundsDoNotCreateFacts(t *testing.T) {
	event := hmc338ResourceEvent()
	line, ok := tracewire.FormatResourceStack(tracewire.ResourceStackRecord{TimestampNS: 10005000000, EventRowID: 1, Event: &event})
	if !ok {
		t.Fatal("invalid fixture")
	}
	for name, raw := range map[string]string{
		"malformed":       line + " ",
		"unknown_version": strings.Replace(line, "/v1", "/v9", 1),
		"prose":           "payload mentions " + line,
	} {
		t.Run(name, func(t *testing.T) {
			if got := renderAttachedTraceSemantics(tracePreviewPart{raw, 1, false}); strings.Contains(got, "source_record") {
				t.Fatal("unadmitted carrier acquired readable facts")
			}
		})
	}
	if got := renderAttachedTraceSemantics(tracePreviewPart{line, 1, true}); got != "" {
		t.Fatal("clipped but syntactically valid record acquired a whole-row display")
	}
	got := renderAttachedTracePreviewBlock(attachedArtifactPreview{head: line + "\n", tail: line + "\n", tailStartLine: 900, elidedBytes: 90000}, "", false)
	rows := hmc338DecodedRows(t, got)
	if len(rows) != 2 || rows[0].Line != 1 || rows[1].Line != 900 || string(rows[0].Record) != string(rows[1].Record) {
		t.Fatal("visible copies were merged or their gutters changed")
	}
	got = renderAttachedTraceSemantics(tracePreviewPart{strings.Repeat(line+"\n", 400), 1, false})
	rows = hmc338DecodedRows(t, got)
	if len(got) > 16<<10 || len(rows) > 32 || !strings.Contains(got, "scan_limited=true") || strings.Contains(got, "omitted_supported_rows=0 ") {
		t.Fatal("whole display/scan budget escaped")
	}

	// A large admitted row is omitted as one unit, not shortened inside its
	// source fields. A later small ordinary semantic row must still fit.
	scalar := tracewire.ResourceScalar{Status: "null"}
	largeText := tracewire.ResourceText{Status: "known", Value: strings.Repeat("\x01", 4000)}
	large, ok := tracewire.FormatResourceStack(tracewire.ResourceStackRecord{EventRowID: 1, Frame: &tracewire.ResourceFrame{
		SourceID: scalar, Depth: scalar, IP: scalar, SymbolID: scalar, FileID: scalar, Offset: scalar, SymbolOffset: scalar,
		VAddr: largeText, Symbol: largeText, Library: largeText,
	}})
	if !ok {
		t.Fatal("large wire fixture not admitted")
	}
	marker := "app-10 (10) [000] .... 1.020000: tracing_mark_write: B|10|keep later business row"
	got = renderAttachedTraceSemantics(tracePreviewPart{large + "\n" + marker + "\n", 33, false})
	rows = hmc338DecodedRows(t, got)
	if len(got) > 16<<10 || len(rows) != 1 || rows[0].Line != 34 || len(rows[0].Record) != 0 || !strings.Contains(got, "omitted_supported_rows=1 ") {
		t.Fatal("oversized native row clipped a field or crowded out a later small semantic row")
	}
	parsed, ok := tracequery.ParseLine(34, marker, nil)
	want, _ := json.Marshal(tracequery.ProjectTraceEventSemantics(parsed))
	if !ok || string(rows[0].Semantics) != string(want) {
		t.Fatal("native display changed an existing semantic row")
	}
	badText, err := tracequery.FormatPerfNAPIAsyncRelation(tracequery.PerfNAPIAsyncRelation{TimestampNS: 21, CPU: 0, TID: 11, PID: 10, TraceID: string([]byte{0xff})})
	if err != nil {
		t.Fatal(err)
	}
	if got := renderAttachedTraceSemantics(tracePreviewPart{badText + "\n", 1, false}); strings.Contains(got, "source_record") {
		t.Fatal("invalid UTF-8 was replaced and presented as byte-exact source text")
	}
}
