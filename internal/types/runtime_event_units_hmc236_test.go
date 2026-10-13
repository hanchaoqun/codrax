package types

import (
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeEventExactMillisecondProjectionHMC236(t *testing.T) {
	for _, tc := range []struct{ ns, ms string }{
		{"9223372036854775807", "9223372036854.775807"},
		{"-9223372036854775808", "-9223372036854.775808"},
		{"9007199254740993", "9007199254.740993"},
		{"-1000001", "-1.000001"}, {"-1", "-0.000001"},
		{"0", "0"}, {"1", "0.000001"}, {"1000", "0.001"}, {"1000000", "1"},
	} {
		t.Run(tc.ns, func(t *testing.T) {
			row := TraceEventSearchInventoryRow{Semantics: &TraceEventSemantics{SchemaVersion: 1, Fields: []TraceEventSemanticField{
				{Key: "source.duration_ns", Type: "int64", Unit: "ns", Status: "known", Value: &tc.ns},
			}}}
			fields, descriptors := runtimeEventNativeFields(row)
			got, found := runtimeEventField(RuntimeEventRow{Fields: fields}, "source.duration_ms")
			if !found || got.Type != "decimal" || got.Unit != "ms" || got.Status != "known" || got.Value == nil || *got.Value != tc.ms {
				t.Fatalf("unit projection lost exact value: %+v", got)
			}
			original, _ := runtimeEventField(RuntimeEventRow{Fields: fields}, "source.duration_ns")
			if original.Value == nil || *original.Value != tc.ns {
				t.Fatalf("unit projection changed original: %+v", original)
			}
			var descriptor RuntimeEventFieldDescriptor
			for _, d := range descriptors {
				if d.Key == "source.duration_ms" {
					descriptor = d
				}
			}
			if descriptor.Type != "decimal" || descriptor.Unit != "ms" || descriptor.Label != "源记录完整区间时长（非窗口内累计值）" {
				t.Fatalf("unit descriptor not tied to original: %+v", descriptor)
			}
		})
	}
}

func TestRuntimeEventUnitProjectionPreservesFieldStatusHMC236(t *testing.T) {
	for _, status := range []string{"unavailable", "invalid", "omitted"} {
		t.Run(status, func(t *testing.T) {
			field := TraceEventSemanticField{Key: "source.start_ns", Type: "int64", Unit: "ns", Status: status, IssueReason: "source_not_available"}
			if status == "omitted" {
				field.Omitted = &TraceEventSemanticOmission{OriginalBytes: 123, SHA256: strings.Repeat("a", 64)}
			}
			fields, _ := runtimeEventNativeFields(TraceEventSearchInventoryRow{Semantics: &TraceEventSemantics{Fields: []TraceEventSemanticField{field}}})
			got, found := runtimeEventField(RuntimeEventRow{Fields: fields}, "source.start_ms")
			if !found || got.Status != field.Status || got.Value != nil || got.IssueReason != field.IssueReason || !reflect.DeepEqual(got.Omitted, field.Omitted) {
				t.Fatalf("unit projection changed uncertainty: %+v", got)
			}
			if got.Omitted != nil && got.Omitted == field.Omitted {
				t.Fatal("unit projection shares original omission pointer")
			}
		})
	}
}

func TestRuntimeEventUnitProjectionIsSelectableAndVerifiedHMC236(t *testing.T) {
	r := runtimeEventRecordHMC236(t)
	c := runtimeEventContractHMC236(t, r)
	s := &AnswerRuntimeMeasurementReceipt{ObservationID: r.ID, View: RuntimeMeasurementMembers, EventSelection: &RuntimeEventSelection{FieldKeys: []string{"jank.start_ts_ns", "jank.start_ts_ms", "jank.reported_duration_ms"}}}
	if !BindRuntimeMeasurementReceipt(s, c) || !s.IsBound() || s.BoundTable.Rows[0][0] != "9007199254740993" || s.BoundTable.Rows[0][1] != "9007199254.740993" {
		t.Fatalf("same-source unit selection failed: %+v", s.BoundTable)
	}
	for _, field := range c.Tables[0].EventData.Fields {
		if field.Key == "jank.appid_ms" || field.Key == "event.trace_time_seconds_ms" {
			t.Fatalf("unregistered unit inferred: %+v", field)
		}
	}
}
