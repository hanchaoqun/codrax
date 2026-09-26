package tracequery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func eventSemanticField(t *testing.T, semantics *types.TraceEventSemantics, key string) types.TraceEventSemanticField {
	t.Helper()
	if semantics != nil {
		for _, field := range semantics.Fields {
			if field.Key == key {
				return field
			}
		}
	}
	t.Fatalf("semantic field %q missing: %+v", key, semantics)
	return types.TraceEventSemanticField{}
}

func expectEventSemanticValue(t *testing.T, semantics *types.TraceEventSemantics, key, value string) {
	t.Helper()
	field := eventSemanticField(t, semantics, key)
	if field.Status != "known" || field.Value == nil || *field.Value != value {
		t.Fatalf("%s got %+v want exact %q", key, field, value)
	}
}

func TestTraceEventSemanticsPublicPluginMarkerCounter(t *testing.T) {
	longName := "business-" + strings.Repeat("part", 160) + "-tail"
	path := writeTraceMarkIntegrityTrace(t, "semantics.trace",
		`hisys-25 (25) [005] .... 1.000000: hi_sysevent: domain=POWER eventname=THERMAL_REPORT type=STAT value=hot level=MINOR`,
		`<hisysevent>-25 (25) [005] .... 1.001000: print: CAMERA_PIPELINE/FRAME_READY: domain=tail_noise name=tail_noise`,
		traceMarkTestLine("writer", 10, 1.002, "B|20|"+longName),
		traceMarkTestLine("writer", 10, 1.003, "E|20"),
		traceMarkTestLine("writer", 10, 1.004, "C|0|HeapSize|0"),
		traceMarkTestLine("writer", 10, 1.005, "C|20|counter|9007199254740993"),
		traceMarkTestLine("writer", 10, 1.006, "C|20|bad|NaN"),
		traceMarkTestLine("writer", 10, 1.007, "G|20|track|async|12"),
		traceMarkTestLine("writer", 10, 1.008, "B|20|jank_event_sync: start_ts=9007199254740993, end_ts=9007199254740995, jank_frames=2, appid=30"),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "event_search", Limit: 40, TimeStart: 1, TimeEnd: 1.01, TimeStartSet: true, TimeEndSet: true}
	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	indexed := Run(idx, q)
	if len(indexed.Events) != 9 || len(streamed.Events) != 9 {
		t.Fatalf("wrong parsed population %d/%d", len(indexed.Events), len(streamed.Events))
	}
	for n := range indexed.Events {
		a, b := indexed.Events[n], streamed.Events[n]
		before, _ := json.Marshal(a)
		sa, sb := ProjectTraceEventSemantics(a.Event), ProjectTraceEventSemantics(b.Event)
		if !types.ValidateTraceEventSemantics(sa) || sa == nil || !reflect.DeepEqual(sa, sb) {
			t.Fatalf("stream/index mismatch row %d: %+v / %+v", n, sa, sb)
		}
		after, _ := json.Marshal(a)
		if string(before) != string(after) {
			t.Fatal("projection mutated event")
		}
		switch n {
		case 0:
			expectEventSemanticValue(t, sa, "plugin.domain", "POWER")
			expectEventSemanticValue(t, sa, "plugin.event_name", "THERMAL_REPORT")
			expectEventSemanticValue(t, sa, "plugin.value", "hot")
			if eventSemanticField(t, sa, "plugin.value").Unit != "" || a.Name != "hi_sysevent" {
				t.Fatal("unit or raw tracepoint changed")
			}
		case 1:
			expectEventSemanticValue(t, sa, "plugin.domain", "CAMERA_PIPELINE")
			expectEventSemanticValue(t, sa, "plugin.event_name", "FRAME_READY")
			expectEventSemanticValue(t, sa, "plugin.contents", "domain=tail_noise name=tail_noise")
			if a.Name != "print" || a.Comm != "<hisysevent>" {
				t.Fatal("business name replaced physical identity")
			}
		case 2:
			expectEventSemanticValue(t, sa, "marker.name", longName)
		case 3:
			if eventSemanticField(t, sa, "marker.name").Status != "unavailable" {
				t.Fatal("end marker invented business name")
			}
		case 4:
			expectEventSemanticValue(t, sa, "marker.payload_pid", "0")
			expectEventSemanticValue(t, sa, "counter.owner_scope", "global")
			expectEventSemanticValue(t, sa, "counter.value", "0")
		case 5:
			expectEventSemanticValue(t, sa, "counter.value", "9007199254740993")
			expectEventSemanticValue(t, sa, "counter.aggregation_status", "excluded")
		case 6:
			if eventSemanticField(t, sa, "counter.value").Status != "invalid" {
				t.Fatal("NaN became numeric")
			}
			expectEventSemanticValue(t, sa, "counter.raw_value", "NaN")
		case 7:
			expectEventSemanticValue(t, sa, "marker.track", "track")
		case 8:
			if a.JankEvent == nil || a.JankEvent.Values == nil || a.JankEvent.Values.StartTSNS != 9007199254740993 {
				t.Fatal("old Jank authority changed")
			}
			for _, field := range sa.Fields {
				if strings.Contains(field.Key, "jank") || field.Key == "source.timestamp_ns" {
					t.Fatal("second Jank authority created")
				}
			}
		}
	}
}

func TestTraceEventSemanticsUnknownAndBoundedProjection(t *testing.T) {
	for _, event := range []Event{{Type: EventSchedSwitch}, {Type: EventHiSystemEvent}, {Type: EventTraceMark}} {
		if ProjectTraceEventSemantics(event) != nil {
			t.Fatalf("unsupported/legacy event got semantics %+v", event)
		}
	}
	legacy := ProjectTraceEventSemantics(Event{Type: EventTraceMark, SpanAction: "C", SpanPID: 0, SpanName: "HeapSize", SpanValue: "0"})
	if eventSemanticField(t, legacy, "counter.value").Status != "unavailable" {
		t.Fatal("legacy fields fabricated parser receipt")
	}
	line := `hisys-25 (25) [005] .... 1.000000: hi_sysevent: domain=POWER value=0`
	event, ok := ParseLine(1, line, nil)
	if !ok {
		t.Fatal("plugin parse failed")
	}
	fields := ProjectTraceEventSemantics(event)
	if eventSemanticField(t, fields, "plugin.event_name").Status != "unavailable" {
		t.Fatal("rawType fallback became business event name")
	}
	expectEventSemanticValue(t, fields, "plugin.event_label", "hi_sysevent")
	expectEventSemanticValue(t, fields, "plugin.value", "0")
	name := strings.Repeat("业务", 800)
	event, ok = ParseLine(2, traceMarkTestLine("writer", 10, 2, "B|20|"+name), nil)
	if !ok {
		t.Fatal("long marker parse failed")
	}
	fields = ProjectTraceEventSemantics(event)
	f := eventSemanticField(t, fields, "marker.name")
	sum := sha256.Sum256([]byte(name))
	if f.Status != "omitted" || f.Value != nil || f.Omitted == nil || f.Omitted.OriginalBytes != len(name) || f.Omitted.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("identity truncated or wrong omission %+v", f)
	}
	control := strings.Repeat("\x00", types.TraceEventSemanticValueByteLimit)
	fields = ProjectTraceEventSemantics(Event{Type: EventHiSystemEvent, PluginFields: &PluginFields{Domain: control, EventName: control, Metric: control, Value: control, Category: control}})
	wire, _ := json.Marshal(fields)
	if fields == nil || !types.ValidateTraceEventSemantics(fields) || len(wire) > types.TraceEventSemanticsByteLimit {
		t.Fatalf("whole budget failed %d %s", len(wire), wire)
	}
	omitted := 0
	for _, field := range fields.Fields {
		if field.Status == "omitted" {
			omitted++
		}
	}
	if omitted == 0 {
		t.Fatal("escaped text did not exercise whole budget")
	}
}

func TestTraceEventSemanticsHiSysSQLKnownEmptyNullAndExactValues(t *testing.T) {
	str := func(v string) *string { return &v }
	integer := func(v int64) *int64 { return &v }
	for _, tc := range []struct {
		name  string
		event tracewire.HiSysEvent
	}{
		{"known_empty_and_null", tracewire.HiSysEvent{TimestampNS: 9007199254740993, Domain: tracewire.HiSysEventName{Name: str(""), Status: "resolved", Reference: integer(0)}, Event: tracewire.HiSysEventName{Status: "null_reference"}, Contents: tracewire.HiSysEventContents{StorageClass: "null"}}},
		{"unresolved_and_blob", tracewire.HiSysEvent{TimestampNS: 0, SourceTID: integer(0), Domain: tracewire.HiSysEventName{Status: "unresolved_reference", Reference: integer(-7)}, Event: tracewire.HiSysEventName{Name: str("event/with\nseparator"), Status: "resolved", Reference: integer(9007199254740993)}, Contents: tracewire.HiSysEventContents{StorageClass: "blob", BytesBase64: "AP8="}}},
		{"invalid_reference_and_integer", tracewire.HiSysEvent{TimestampNS: 1, Domain: tracewire.HiSysEventName{Status: "invalid_reference_storage_class"}, Event: tracewire.HiSysEventName{Name: str("name"), Status: "resolved", Reference: integer(1)}, Contents: tracewire.HiSysEventContents{StorageClass: "integer", Text: str("9007199254740993")}}},
		{"empty_text", tracewire.HiSysEvent{TimestampNS: 1, Domain: tracewire.HiSysEventName{Status: "null_reference"}, Event: tracewire.HiSysEventName{Status: "null_reference"}, Contents: tracewire.HiSysEventContents{StorageClass: "text", Text: str("")}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := tracewire.FormatHiSysEventObservation(tc.event)
			if err != nil {
				t.Fatal(err)
			}
			event, ok := ParseLine(1, wire, nil)
			if !ok || event.Type != EventHiSystemEvent {
				t.Fatalf("real SQL observation did not parse %+v %v", event, ok)
			}
			fields := ProjectTraceEventSemantics(event)
			if fields == nil || !types.ValidateTraceEventSemantics(fields) {
				t.Fatalf("projection invalid %+v", fields)
			}
			if event.PID != 0 || event.CPU != -1 {
				t.Fatal("source record acquired scheduler owner")
			}
			expectEventSemanticValue(t, fields, "source.representation", "sql_hisysevent")
			if tc.event.Domain.Name != nil {
				expectEventSemanticValue(t, fields, "plugin.domain", *tc.event.Domain.Name)
			} else if field := eventSemanticField(t, fields, "plugin.domain"); field.Value != nil || field.IssueReason != tc.event.Domain.Status {
				t.Fatalf("unknown identity guessed %+v", field)
			}
			if tc.event.SourceTID == nil {
				if eventSemanticField(t, fields, "source.tid").Value != nil {
					t.Fatal("NULL tid became 0")
				}
			} else {
				expectEventSemanticValue(t, fields, "source.tid", "0")
			}
			if tc.event.Contents.Text != nil {
				expectEventSemanticValue(t, fields, "source.contents", *tc.event.Contents.Text)
			}
			if tc.event.Contents.StorageClass == "blob" {
				expectEventSemanticValue(t, fields, "source.contents_base64", "AP8=")
			}
			encoded, _ := json.Marshal(fields)
			var restored types.TraceEventSemantics
			if err := json.Unmarshal(encoded, &restored); err != nil || !reflect.DeepEqual(fields, &restored) {
				t.Fatal("exact semantic JSON roundtrip changed")
			}
			if tc.event.TimestampNS == 9007199254740993 {
				expectEventSemanticValue(t, fields, "source.timestamp_ns", "9007199254740993")
			}
		})
	}
}
