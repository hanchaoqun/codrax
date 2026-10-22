package tracequery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC280ClockSyncRealWitnessPublicIndexedStreaming(t *testing.T) {
	path := filepath.Join("testdata", "hmc280_clock_sync.systrace")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{View: "event_search", TimeStart: 170441.3733, TimeEnd: 170441.3734, Limit: 10}
	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range []Result{Run(idx, q), streamed} {
		if len(result.Events) != 2 {
			t.Fatalf("lost witnesses: %+v", result)
		}
		for i, row := range result.Events {
			sem := ProjectTraceEventSemantics(row.Event)
			if !types.ValidateTraceEventSemantics(sem) || sem == nil || row.Type != EventUnknown || row.SpanAction != "" {
				t.Fatalf("invalid witness or new duration authority: %+v", row)
			}
			expectEventSemanticValue(t, sem, "clock_sync.mapping_status", "unmapped_observation_only")
			if i == 0 {
				expectEventSemanticValue(t, sem, "clock_sync.trace_time_ns", "170441373302000")
				expectEventSemanticValue(t, sem, "clock_sync.realtime_ms", "1787277345616")
			} else {
				expectEventSemanticValue(t, sem, "clock_sync.trace_time_ns", "170441373322000")
				expectEventSemanticValue(t, sem, "clock_sync.parent_seconds", "10143.297852")
			}
			for _, key := range []string{"clock_sync.offset_ns", "clock_sync.drift", "clock_sync.error_bound_ns", "clock_sync.valid_interval"} {
				if field := eventSemanticField(t, sem, key); field.Status != "unavailable" || field.Value != nil {
					t.Fatalf("invented calibration %s: %+v", key, field)
				}
			}
			encoded, _ := json.Marshal(row.Event)
			var restored Event
			if json.Unmarshal(encoded, &restored) != nil || !reflect.DeepEqual(sem, ProjectTraceEventSemantics(restored)) {
				t.Fatal("cached witness changed")
			}
		}
	}
	q.TimeStart, q.TimeEnd = 170441.37331, 170441.37333
	if result := Run(idx, q); len(result.Events) != 1 || result.Events[0].Line != 4 {
		t.Fatal("point boundaries paired two independent records")
	}
	q.TimeStart, q.TimeEnd = 170442, 170443
	if result := Run(idx, q); len(result.Events) != 0 {
		t.Fatal("empty window expanded")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("query changed source")
	}
}

func TestHMC280ClockSyncStrictWirePrecisionAndNegativeControls(t *testing.T) {
	for _, tc := range []struct{ payload, key, value, status string }{
		{"realtime_ts=0", "clock_sync.realtime_ms", "0", "known"},
		{"realtime_ts=9007199254740993", "clock_sync.realtime_ms", "9007199254740993", "known"},
		{"parent_ts=0.000000", "clock_sync.parent_seconds", "0.000000", "known"},
		{"parent_ts=10143.297852000", "clock_sync.parent_seconds", "10143.297852000", "known"},
		{"realtime_ts=9223372036854775808", "clock_sync.realtime_ms", "", "invalid"},
		{"realtime_ts=1.5", "clock_sync.realtime_ms", "", "invalid"},
		{"parent_ts=1e3", "clock_sync.parent_seconds", "", "invalid"},
		{"parent_ts=-1", "clock_sync.parent_seconds", "", "invalid"},
		{"realtime_ts=3 realtime_ts=4", "clock_sync.realtime_ms", "", "invalid"},
		{"realtime_ts=3 ", "clock_sync.realtime_ms", "", "invalid"},
		{"unknown_ts=1", "clock_sync.reference_value", "", "invalid"},
		{strings.Repeat("x", 129), "clock_sync.reference_value", "", "invalid"},
	} {
		t.Run(tc.payload, func(t *testing.T) {
			line := "hitrace-5 (5) [002] .... 9007199.254740993: tracing_mark_write: " + clockSyncPrefix + tc.payload
			event, ok := ParseLine(1, line, nil)
			if !ok {
				t.Fatal("original record lost")
			}
			sem := ProjectTraceEventSemantics(event)
			field := eventSemanticField(t, sem, tc.key)
			if field.Status != tc.status || tc.status == "known" && (field.Value == nil || *field.Value != tc.value) {
				t.Fatalf("wrong field: %+v", field)
			}
			expectEventSemanticValue(t, sem, "clock_sync.trace_time_ns", "9007199254740993")
		})
	}
	for _, payload := range []string{"ordinary prose", "mention " + clockSyncPrefix + "realtime_ts=1", "B|5|" + clockSyncPrefix + "realtime_ts=1", "I|5|" + clockSyncPrefix + "parent_ts=1", "trace_event_clock_syncer: realtime_ts=1"} {
		event, ok := ParseLine(1, "app-5 (5) [002] .... 1.000000: tracing_mark_write: "+payload, nil)
		if !ok {
			t.Fatal("original event lost")
		}
		if event.PluginFields != nil && event.PluginFields.ClockSync != nil {
			t.Fatalf("ordinary marker promoted: %q", payload)
		}
	}
	event, _ := ParseLine(1, "app-5 (5) [002] .... 1.000000: foreign_event: "+clockSyncPrefix+"realtime_ts=1", nil)
	if event.PluginFields != nil && event.PluginFields.ClockSync != nil {
		t.Fatal("foreign event minted clock witness")
	}
}
