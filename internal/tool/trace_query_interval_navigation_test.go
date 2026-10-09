package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceIntervalNavigationDefaultScopeDoesNotEraseSelectors(t *testing.T) {
	for _, tc := range []struct {
		name, original, effective string
		absent                    bool
	}{
		{"injected", `{}`, `{"view":"process_measurements","target_scope":"process","pid":0,"thread":""}`, true},
		{"explicit", `{"target_scope":"process"}`, `{"view":"process_measurements","target_scope":"process","pid":0,"thread":""}`, false},
		{"inherited owner", `{}`, `{"view":"process_measurements","target_scope":"process","pid":7,"thread":""}`, false},
		{"thread", `{}`, `{"view":"event_search","target_scope":"thread","pid":0,"thread":"worker"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var p traceQueryParams
			if err := json.Unmarshal([]byte(tc.effective), &p); err != nil {
				t.Fatal(err)
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal(traceQueryIntervalNavigationParams(p, []byte(tc.effective), []byte(tc.original)), &got); err != nil {
				t.Fatal(err)
			}
			_, scope := got["target_scope"]
			if scope == tc.absent {
				t.Fatal("wrong scope normalization", string(got["target_scope"]))
			}
			var before map[string]json.RawMessage
			_ = json.Unmarshal([]byte(tc.effective), &before)
			for key, value := range before {
				if key != "target_scope" && string(got[key]) != string(value) {
					t.Fatalf("lost selector %s", key)
				}
			}
		})
	}
}

func TestTraceIntervalNavigationTypedFamilies(t *testing.T) {
	for _, tc := range []struct {
		family tracequery.EventType
		want   string
	}{
		{tracequery.EventMeasureInterval, tracequery.ViewMeasurements},
		{tracequery.EventProcessMeasureInterval, tracequery.ViewProcessMeasurements},
		{tracequery.EventCPUMeasureInterval, tracequery.ViewCPUStateFrequency},
		{tracequery.EventCPUFrequency, ""},
		{tracequery.EventTraceMark, ""},
	} {
		result := tracequery.Result{View: "event_search", Events: []tracequery.EventView{{Event: tracequery.Event{Type: tc.family}}, {Event: tracequery.Event{Type: tc.family}}}}
		views := traceQueryNativeIntervalViews(result)
		if tc.want == "" && len(views) != 0 || tc.want != "" && (len(views) != 1 || views[0] != tc.want) {
			t.Fatal(tc, views)
		}
		var b strings.Builder
		writeTraceNativeIntervalNavigation(&b, result)
		if tc.want == "" && b.Len() != 0 || tc.want != "" && !strings.Contains(b.String(), tc.want) {
			t.Fatal(tc, b.String())
		}
	}
}
