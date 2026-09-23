package tracequery

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestTraceCapabilityCatalogEngineAndWireParity(t *testing.T) {
	catalog, err := TraceCapabilities("", true)
	if err != nil {
		t.Fatal(err)
	}
	views, metrics := map[string]bool{}, map[string]bool{}
	for _, m := range catalog.Metrics {
		if metrics[m.ID] {
			t.Fatalf("duplicate metric %s", m.ID)
		}
		metrics[m.ID] = true
		if len(m.Outputs) == 0 || len(m.Requirements.Conditions) == 0 || len(m.Limitations) == 0 {
			t.Errorf("incomplete metric %+v", m)
		}
		for _, output := range m.Outputs {
			for _, field := range output.Fields {
				if _, ok := capabilityJSONPath(reflect.TypeOf(Result{}), strings.Split(output.Section+"."+field, ".")); !ok {
					t.Errorf("metric %s refers to nonexistent Result wire field %s.%s", m.ID, output.Section, field)
				}
			}
			if output.Unit == "" || output.Caliber == "" {
				t.Errorf("unit/caliber missing: %+v", output)
			}
		}
	}
	for _, v := range catalog.Views {
		if views[v.View] {
			t.Fatalf("duplicate view %s", v.View)
		}
		views[v.View] = true
		for _, ref := range v.MetricRefs {
			if !metrics[ref] {
				t.Errorf("%s references absent metric %s", v.View, ref)
			}
		}
	}
	for _, v := range catalog.Views {
		for _, component := range v.Components {
			if !views[component] {
				t.Errorf("%s references absent component %s", v.View, component)
			}
		}
	}
	names := CapabilityViewNames()
	sort.Strings(names)
	if !reflect.DeepEqual(names, CanonicalViewNames()) {
		t.Fatalf("catalog/capacity drift: %v", names)
	}
	if _, err := TraceCapabilities("causal_impact", true); err == nil {
		t.Fatal("tool alias unexpectedly acquired engine meaning")
	}
}

// Follow actual serialized struct fields, including embedded structs and slices;
// the metadata cannot silently invent output paths while tests only pin prose.
func capabilityJSONPath(typ reflect.Type, path []string) (reflect.Type, bool) {
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice {
		typ = typ.Elem()
	}
	if len(path) == 0 {
		return typ, true
	}
	if typ.Kind() != reflect.Struct {
		return nil, false
	}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.PkgPath != "" {
			continue
		}
		tag := strings.Split(f.Tag.Get("json"), ",")[0]
		if tag == "-" {
			continue
		}
		if tag == path[0] {
			return capabilityJSONPath(f.Type, path[1:])
		}
		if f.Anonymous && tag == "" {
			if found, ok := capabilityJSONPath(f.Type, path); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func TestTraceCapabilityCatalogIsolationAndCompositeDetail(t *testing.T) {
	first, _ := TraceCapabilities("frame_root_cause_bundle", true)
	before, _ := json.Marshal(first)
	foundIO := false
	for _, metric := range first.Metrics {
		if metric.ID == "io_request_latency" {
			foundIO = true
		}
	}
	if !foundIO {
		t.Fatal("composite detail lost the referenced window_stats IO contract")
	}
	first.Views[0].InputFormats[0] = "mutated"
	first.Metrics[0].Requirements.Conditions[0] = "mutated"
	first.Metrics[0].Outputs[0].Fields[0] = "mutated"
	second, _ := TraceCapabilities("frame_root_cause_bundle", true)
	after, _ := json.Marshal(second)
	if string(before) != string(after) {
		t.Fatal("catalog reads share mutable descriptor storage")
	}
	if second.QueryTimeUnit != "seconds" || !strings.Contains(second.QueryTimeAxis, "nanoseconds on that same axis") {
		t.Fatal("query time and native payload units conflated")
	}
}
