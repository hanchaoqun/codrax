package tracequery

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestHMC226NativeTargetTimelineReusesMeasuredPartition(t *testing.T) {
	content, err := os.ReadFile("../../eval/fixtures/hmosperf_dual_source_windows/baseline.systrace")
	if err != nil {
		t.Fatal(err)
	}
	idx := buildTraceIndex(t, "baseline.systrace", string(content))
	for _, view := range []string{"wakeup_chain", "window_stats", "thread_timeline"} {
		t.Run(view, func(t *testing.T) {
			q := Query{View: view, PID: 41, TimeStart: 1, TimeEnd: 1.01, Limit: 20}
			result := Run(idx, q)
			provider, ok := any(result).(interface{ NativeTargetTimeline() *TimelineResult })
			if !ok || provider.NativeTargetTimeline() == nil {
				t.Fatal("measured target partition is discarded before native fact publication")
			}
			timeline := provider.NativeTargetTimeline()
			q.View = "thread_timeline"
			want := Run(idx, q).Timeline
			if !reflect.DeepEqual(timeline, want) {
				t.Fatalf("native display reconstructed the measured partition: got=%+v want=%+v", timeline, want)
			}
			if view != "thread_timeline" {
				if result.Timeline != nil {
					t.Fatal("new display retention activated the legacy timeline output branch")
				}
				raw, _ := json.Marshal(result)
				var wire map[string]json.RawMessage
				json.Unmarshal(raw, &wire)
				if _, present := wire["timeline"]; present {
					t.Fatal("display retention duplicated the legacy wire payload")
				}
			}
		})
	}
	for _, q := range []Query{
		{View: "event_search", PID: 41, Limit: 20},
		{View: "window_stats", PID: 999, TimeStart: 1, TimeEnd: 1.01},
	} {
		result := Run(idx, q)
		if provider, ok := any(result).(interface{ NativeTargetTimeline() *TimelineResult }); ok && provider.NativeTargetTimeline() != nil {
			t.Fatalf("absent/unanalyzed target minted display facts: %+v", q)
		}
	}
}
