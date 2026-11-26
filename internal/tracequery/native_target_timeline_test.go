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
	for _, view := range []string{"wakeup_chain", "window_stats", "thread_timeline", "frame_root_cause_bundle"} {
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

func TestFrameRootCauseBundleCarriesWakeupIPCEvidence(t *testing.T) {
	idx := buildTraceIndex(t, "binder.systrace", donghuP9TrueBinderWaitTrace)
	q := Query{View: "wakeup_chain", PID: 17267, TimeStart: idx.FirstTs, TimeEnd: idx.LastTs}
	ipcEvidence := func(result Result) []EvidenceFact {
		var out []EvidenceFact
		for _, fact := range result.EvidencePack {
			if fact.Predicate == "binder_ipc" {
				out = append(out, fact)
			}
		}
		return out
	}
	want := ipcEvidence(Run(idx, q))
	if len(want) == 0 {
		t.Fatal("fixture must publish Binder evidence")
	}
	q.View = "frame_root_cause_bundle"
	if got := ipcEvidence(Run(idx, q)); !reflect.DeepEqual(got, want) {
		t.Fatalf("bundle Binder evidence differs from wakeup_chain: got=%+v want=%+v", got, want)
	}
}
