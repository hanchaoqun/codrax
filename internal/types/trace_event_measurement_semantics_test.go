package types

import "testing"

func TestMeasurementSemanticRegistryFamilyAndEnumBoundaries(t *testing.T) {
	field := func(key, value string) TraceEventSemanticField {
		d, ok := LookupTraceEventSemanticDescriptor(key)
		if !ok {
			t.Fatal(key)
		}
		return TraceEventSemanticField{Key: key, Type: d.Type, Unit: d.Unit, Status: "known", Value: semanticTestValue(value)}
	}
	good := &TraceEventSemantics{SchemaVersion: 1, Fields: []TraceEventSemanticField{
		field("source.filter_table", "measure_filter"), field("source.filter_id", "9007199254740993"),
		field("source.filter_storage_class", "integer"), field("source.filter_encoding", ""), field("source.filter_status", "observed_unique"),
		field("plugin.value_storage_class", "text"), field("plugin.value_encoding", "base64"), field("plugin.value", "gP8"),
	}}
	if !ValidateTraceEventSemantics(good) || !traceEventSemanticsMatchEventType(good, "measure_interval") {
		t.Fatal("valid display refused")
	}
	for _, family := range []string{"process_measure_interval", "trace_mark", "cpu_measure_interval", "hi_sysevent", "gpu_state"} {
		if traceEventSemanticsMatchEventType(good, family) {
			t.Fatal("family borrowed metadata", family)
		}
	}
	for key, bad := range map[string]string{"source.filter_storage_class": "number", "source.filter_status": "verified_gpu", "source.filter_table": "cpu_measure_filter", "plugin.value_encoding": "decoded", "plugin.value_storage_class": "Hz"} {
		copy := CloneTraceEventSemantics(good)
		for i := range copy.Fields {
			if copy.Fields[i].Key == key {
				copy.Fields[i].Value = semanticTestValue(bad)
			}
		}
		if ValidateTraceEventSemantics(copy) {
			t.Fatal("invalid enum accepted", key, bad)
		}
	}
	for i := range good.Fields {
		if good.Fields[i].Key == "source.filter_table" {
			good.Fields[i].Value = semanticTestValue("process_measure_filter")
		}
	}
	if traceEventSemanticsMatchEventType(good, "measure_interval") {
		t.Fatal("cross-registry identity accepted")
	}
}
