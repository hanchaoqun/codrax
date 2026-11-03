package tracequery

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestHMC338SourceRecordPreviewIsNotAnAdmissionOrMutation(t *testing.T) {
	for _, event := range []Event{
		{Type: EventResourceStack, FieldText: "# codrax_resource_stack/v1 record=broken"},
		{Type: EventCPUMeasureInterval, FieldText: "# codrax_cpu_measure_interval/v1 record=broken"},
		{Type: EventFrameGPU},
		{Type: EventUnknown, FieldText: "# codrax_frame_gpu/v1 ts_ns=1 gpu_row=0 frame_row=0 duration_ns=0"},
		{Type: EventTraceDBRecord, PluginFields: &PluginFields{TraceDBRecord: &TraceDBRecordFields{Payload: []byte("private full SQL archive bytes")}}},
	} {
		if got := ProjectTraceEventSourceRecordPreview(event); len(got.Fields) != 0 || got.TimestampNS != "" || got.TimeRole != "" {
			t.Fatalf("unavailable/opaque source acquired a record: %+v", got)
		}
	}
	line, err := FormatPerfNAPIAsyncRelation(PerfNAPIAsyncRelation{TimestampNS: 9007199254740993, CPU: 0, TID: 12, PID: 11, TraceID: "source text"})
	if err != nil {
		t.Fatal(err)
	}
	event, ok := ParseLine(71, line, nil)
	if !ok {
		t.Fatal("source not parsed")
	}
	before, _ := json.Marshal(event)
	p := ProjectTraceEventSourceRecordPreview(event)
	if p.TimestampNS != "9007199254740993" || !strings.Contains(string(p.Fields), `"traceid":"source text"`) {
		t.Fatal("exact source values lost")
	}
	for n := range p.Fields {
		p.Fields[n] = 'x'
	}
	after, _ := json.Marshal(event)
	if string(before) != string(after) || !reflect.DeepEqual(ProjectTraceEventSourceRecordPreview(event), ProjectTraceEventSourceRecordPreview(event)) {
		t.Fatal("returned display bytes mutated the event or were unstable")
	}
	// Unknown source time stays empty; a known negative start is not replaced
	// by the wire's nonnegative ordering coordinate.
	negative, duration, value := int64(-12), int64(1), int64(0)
	for _, start := range []*int64{nil, &negative} {
		r := tracewire.CPUMeasureInterval{RowID: 0, FilterID: 0, CPU: 0, Kind: "idle", Encoding: "native_sql_idle", StartNS: start, DurationNS: &duration, Value: &value}
		if start == nil {
			r.Issue = "invalid_timestamp"
		}
		line, err := tracewire.FormatCPUMeasureInterval(r)
		if err != nil {
			t.Fatal(err)
		}
		event, ok := ParseLine(1, line, nil)
		if !ok {
			t.Fatal("native source row missing")
		}
		got := ProjectTraceEventSourceRecordPreview(event)
		want := ""
		if start != nil {
			want = "-12"
		}
		if got.TimestampNS != want {
			t.Fatal("ordering timestamp promoted to original source time")
		}
	}
}
