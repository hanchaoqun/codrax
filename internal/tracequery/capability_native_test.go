package tracequery

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNativeIntervalNavigationCatalog(t *testing.T) {
	for view, family := range map[string]string{
		ViewMeasurements: "measure_interval", ViewProcessMeasurements: "process_measure_interval", ViewCPUStateFrequency: "cpu_measure_interval",
	} {
		catalog, err := TraceCapabilities(view, true)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(catalog)
		if !strings.Contains(string(b), `"native_interval_sources":[`) || !strings.Contains(string(b), `"event_type":"`+family+`"`) {
			t.Errorf("%s lacks source-family navigation: %s", view, b)
		}
		if !catalog.StaticOnly || catalog.Evidence || catalog.CaptureAvailability != "not_evaluated" {
			t.Fatal("navigation granted capture authority")
		}
	}
}

func TestNativeIntervalNavigationStrictRegistry(t *testing.T) {
	original := NativeIntervalNavigations()
	if len(original) != 3 {
		t.Fatal(original)
	}
	copy := NativeIntervalNavigations()
	copy[0].View = "root_cause_rank"
	if !reflect.DeepEqual(original, NativeIntervalNavigations()) {
		t.Fatal("shared registry mutated")
	}
	for _, n := range original {
		byEvent, ok := NativeIntervalNavigationForEvent(n.EventType)
		byTable, tableOK := NativeIntervalNavigationForCoverage(n.CoverageFamily, n.SourceTable)
		if !ok || !tableOK || byEvent != n || byTable != n {
			t.Fatal(n, byEvent, byTable)
		}
		if _, ok := NativeIntervalNavigationForEvent(EventType(strings.ToUpper(string(n.EventType)))); ok {
			t.Fatal("name inference")
		}
		catalog, err := TraceCapabilities(n.View, true)
		if err != nil || !reflect.DeepEqual(catalog.Views[0].NativeIntervalSources, []NativeIntervalNavigation{n}) {
			t.Fatal("catalog drift", err)
		}
		catalog.Views[0].NativeIntervalSources[0].View = "changed"
		if got, _ := NativeIntervalNavigationForEvent(n.EventType); got != n {
			t.Fatal("catalog mutation leaked")
		}
		found := false
		for _, m := range catalog.Metrics {
			for _, f := range m.Requirements.AllOf {
				found = found || f == string(n.EventType)
			}
			for _, alternative := range m.Requirements.AnyOf {
				for _, f := range alternative {
					found = found || f == string(n.EventType)
				}
			}
		}
		if !found {
			t.Fatal("navigation family lacks metric requirement", n)
		}
	}
	for _, name := range []EventType{"gpufreq", "gpu_state", "measure", EventTraceMark, EventUnknown} {
		if _, ok := NativeIntervalNavigationForEvent(name); ok {
			t.Fatal("raw name promoted", name)
		}
	}
	if _, ok := NativeIntervalNavigationForCoverage("measurement", "process_measure"); ok {
		t.Fatal("registry borrowed across tables")
	}
}

func TestMeasurementSemanticProjectionStorageAndIdentityMatrix(t *testing.T) {
	for _, value := range []tracewire.MeasureScalar{
		{StorageClass: "integer", Value: "9007199254740993"}, {StorageClass: "real", Value: "1.5"},
		{StorageClass: "text", Value: "1"}, {StorageClass: "text", Value: ""}, {StorageClass: "blob", Value: "ADE"},
		{StorageClass: "text", Value: base64.RawStdEncoding.EncodeToString([]byte{0x80, 0xff}), Encoding: "base64"},
		{StorageClass: "null"}, {StorageClass: "absent"},
	} {
		t.Run(value.StorageClass+"_"+value.Value, func(t *testing.T) {
			r := measurementTestRecord(1, -5, 10)
			r.Value, r.MeasureType, r.Filter.Type, r.Filter.SourceArgSetID = value, value, value, value
			event, ok := ParseLine(1, strings.TrimSpace(measurementTestText(t, r)), nil)
			if !ok {
				t.Fatal("fixture")
			}
			sem := ProjectTraceEventSemantics(event)
			if sem == nil || !types.ValidateTraceEventSemantics(sem) || len(sem.Fields) > 32 {
				t.Fatal("semantic loss", sem)
			}
			for _, pair := range [][3]string{{"plugin.value", "plugin.value_storage_class", "plugin.value_encoding"}, {"plugin.category", "plugin.category_storage_class", "plugin.category_encoding"}, {"plugin.filter_type", "plugin.filter_type_storage_class", "plugin.filter_type_encoding"}, {"source.arg_set_id", "source.arg_set_storage_class", "source.arg_set_encoding"}} {
				expectEventSemanticValue(t, sem, pair[1], value.StorageClass)
				encoding := value.Encoding
				if value.StorageClass == "blob" {
					encoding = "base64"
				}
				expectEventSemanticValue(t, sem, pair[2], encoding)
				if value.StorageClass != "null" && value.StorageClass != "absent" {
					expectEventSemanticValue(t, sem, pair[0], value.Value)
				}
			}
		})
	}
	for _, status := range []string{"unknown", "ambiguous"} {
		r := measurementTestRecord(1, 1, 1)
		r.Filter, r.FilterStatus = nil, status
		e, _ := ParseLine(1, strings.TrimSpace(measurementTestText(t, r)), nil)
		s := ProjectTraceEventSemantics(e)
		expectEventSemanticValue(t, s, "source.filter_status", status)
		expectEventSemanticValue(t, s, "source.filter_id", "-5")
		for _, f := range s.Fields {
			if f.Key == "source.arg_set_id" {
				t.Fatal("unknown filter borrowed metadata")
			}
		}
	}
	for _, id := range []int64{10, 20} {
		r := measurementTestRecord(id, 1, 1)
		r.FilterID = measurementTestInt(id)
		r.Filter.ID = r.FilterID
		e, _ := ParseLine(1, strings.TrimSpace(measurementTestText(t, r)), nil)
		s := ProjectTraceEventSemantics(e)
		expectEventSemanticValue(t, s, "plugin.metric", "gpufreq")
		expectEventSemanticValue(t, s, "source.filter_id", r.FilterID.Value)
	}
}

func TestProcessMeasurementSemanticProjectionPreservesRawTypes(t *testing.T) {
	r := processMeasureTestRecord(1, -5, 10, 1, 101)
	r.Value = tracewire.ProcessMeasureScalar{Status: "invalid_storage", StorageClass: "text", Value: "1"}
	e, ok := ParseLine(1, strings.TrimSpace(processMeasureTestText(t, r)), nil)
	if !ok {
		t.Fatal("fixture")
	}
	s := ProjectTraceEventSemantics(e)
	expectEventSemanticValue(t, s, "source.filter_id", "1")
	expectEventSemanticValue(t, s, "source.filter_table", "process_measure_filter")
	expectEventSemanticValue(t, s, "plugin.value_storage_class", "text")
	expectEventSemanticValue(t, s, "counter.raw_value", "1")
	for _, f := range s.Fields {
		if f.Key == "source.filter_status" || f.Key == "source.arg_set_id" {
			t.Fatal("borrowed generic metadata")
		}
	}
}

func TestMeasurementSemanticProjectionPreservesRawReferences(t *testing.T) {
	r := measurementTestRecord(1, -5, 10)
	r.Value.StorageClass, r.Value.Value = "text", "1"
	line := strings.TrimSpace(measurementTestText(t, r))
	event, ok := ParseLine(1, line, nil)
	if !ok {
		t.Fatal("fixture")
	}
	sem := ProjectTraceEventSemantics(event)
	for key, want := range map[string]string{
		"source.filter_id": "-5", "source.filter_storage_class": "integer", "source.filter_table": "measure_filter",
		"source.filter_status": "observed_unique", "source.arg_set_id": "99", "source.arg_set_storage_class": "integer",
		"plugin.value_storage_class": "text", "plugin.value_encoding": "", "plugin.value": "1",
		"plugin.filter_type": "measure_filter", "plugin.filter_type_storage_class": "text", "plugin.category_storage_class": "text",
	} {
		expectEventSemanticValue(t, sem, key, want)
	}
}
