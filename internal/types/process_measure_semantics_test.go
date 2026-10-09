package types

import "testing"

func TestProcessMeasurementSemanticsExactFamily(t *testing.T) {
	p := &TraceEventSemantics{SchemaVersion: 1}
	for _, kv := range [][2]string{{"source.representation", "sql_process_measure_interval"}, {"source.table", "process_measure"}, {"source.subject_role", "process_measurement_not_thread_execution"}, {"counter.aggregation_status", "not_aggregated_unit_and_quantity_semantics_unknown"}, {"source.start_ns", "-1"}, {"plugin.metric", "RSS"}, {"plugin.value", "9223372036854775807"}} {
		d, ok := LookupTraceEventSemanticDescriptor(kv[0])
		if !ok {
			t.Fatal(kv)
		}
		v := kv[1]
		p.Fields = append(p.Fields, TraceEventSemanticField{Key: kv[0], Type: d.Type, Unit: d.Unit, Status: "known", Value: &v})
	}
	if !ValidateTraceEventSemantics(p) || !traceEventSemanticsMatchEventType(p, "process_measure_interval") {
		t.Fatal("native process semantics unavailable", p)
	}
	for _, typ := range []string{"trace_mark", "hi_sysevent", "cpu_measure_interval", "sched_switch"} {
		if traceEventSemanticsMatchEventType(p, typ) {
			t.Fatal("process semantic authority crossed event family", typ)
		}
	}
	for _, key := range []string{"source.execution_cpu", "source.emitter_tid", "marker.payload_pid", "resource.size", "counter.value"} {
		bad := CloneTraceEventSemantics(p)
		d, _ := LookupTraceEventSemanticDescriptor(key)
		v := "0"
		bad.Fields = append(bad.Fields, TraceEventSemanticField{Key: key, Type: d.Type, Unit: d.Unit, Status: "known", Value: &v})
		if traceEventSemanticsMatchEventType(bad, "process_measure_interval") {
			t.Fatal("foreign semantic admitted", key)
		}
	}
	bad := CloneTraceEventSemantics(p)
	bad.Fields[0].Value = semanticTestValue("parsed_trace_marker")
	if traceEventSemanticsMatchEventType(bad, "process_measure_interval") {
		t.Fatal("wrong source representation accepted")
	}
}
