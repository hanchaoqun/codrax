package tracequery

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// The reporting event is not an endpoint of the reported symptom. Its later
// header time does not turn a parser-defined source-clock field into a new clock.
func TestJankSourceClockPublicIndexedAndStreamed(t *testing.T) {
	path := writeTraceMarkIntegrityTrace(t, "source-clock.systrace",
		traceMarkTestLine("writer", 10, 3, "B|20|jank_event_sync: start_ts=1000000001, end_ts=1070000001, jank_frames=7, appid=30"),
		traceMarkTestLine("writer", 10, 3.125, "E|20"),
		traceMarkTestLine("writer", 10, 4, "B|20|jank_event_sync: start_ts=9007199254740993, end_ts=9007199254741093, jank_frames=2, appid=30"),
		traceMarkTestLine("writer", 10, 5, "B|20|other jank_event_sync: start_ts=0, end_ts=1, jank_frames=99, appid=30"),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	queries := []struct {
		name  string
		query Query
		lines []int
	}{
		{"all", Query{View: "event_search", Limit: 40, EventFieldFilters: []EventFieldFilter{{Field: "jank_frames", Op: "gte", Value: "2"}}}, []int{1, 3}},
		{"header_window", Query{View: "event_search", Limit: 40, TimeStart: 2.9, TimeEnd: 3.1, TimeStartSet: true, TimeEndSet: true, EventFieldFilters: []EventFieldFilter{{Field: "jank_frames", Op: "gte", Value: "2"}}}, []int{1}},
		{"payload_range", Query{View: "event_search", Limit: 40, EventFieldFilters: []EventFieldFilter{{Field: "start_ts", Op: "gte", Value: "9007199254740993"}, {Field: "end_ts", Op: "lte", Value: "9007199254741093"}}}, []int{3}},
		{"one_ns_exclusion", Query{View: "event_search", Limit: 40, EventFieldFilters: []EventFieldFilter{{Field: "start_ts", Op: "gt", Value: "9007199254740993"}}}, nil},
	}
	for _, tc := range queries {
		t.Run(tc.name, func(t *testing.T) {
			streamed, err := StreamEventSearch(context.Background(), path, tc.query)
			if err != nil {
				t.Fatal(err)
			}
			for lane, result := range map[string]Result{"indexed": Run(idx, tc.query), "streamed": streamed} {
				if got := eventViewLines(result.Events); !reflect.DeepEqual(got, tc.lines) && !(len(got) == 0 && len(tc.lines) == 0) {
					t.Fatalf("%s wrong numeric/header scope: %v", lane, got)
				}
				for _, ev := range result.Events {
					if ev.JankEvent == nil || ev.JankEvent.Values == nil {
						t.Fatal("native fields missing")
					}
					if ev.JankEvent.TimeDomainStatus != "source_trace_clock" {
						t.Errorf("%s parser-defined source clock marked %q", lane, ev.JankEvent.TimeDomainStatus)
					}
					if ev.PID != 10 || ev.SpanPID != 20 || ev.JankEvent.Values.AppID != 30 {
						t.Fatal("clock metadata changed identities")
					}
					if ev.Line == 1 && (ev.Ts != 3 || ev.JankEvent.Values.ReportedDurationNS != 70000000) {
						t.Fatal("header or exact duration changed")
					}
					summary := JankEventSummary(ev.Event)
					if !strings.Contains(summary, "native_time_domain=source_trace_clock") || strings.Contains(summary, "no header-clock alignment") {
						t.Errorf("%s contradictory summary: %s", lane, summary)
					}
					if ev.Line == 3 {
						wire, err := json.Marshal(ev.JankEvent)
						if err != nil || !strings.Contains(string(wire), `"start_ts_ns":9007199254740993`) || !strings.Contains(string(wire), `"reported_duration_ns":100`) {
							t.Fatalf("native int64 lost: %s %v", wire, err)
						}
					}
				}
			}
		})
	}
	spans := Run(idx, Query{View: "span_window", SpanName: "jank_event_sync", Limit: 40}).SpanWindows
	if len(spans) == 0 || spans[0].StartTs != 3 || spans[0].EndTs != 3.125 || spans[0].DurationMs != 125 {
		t.Fatalf("payload endpoints replaced B/E span: %+v", spans)
	}
}

func TestJankSourceClockPublicExactGrammarBoundary(t *testing.T) {
	for _, payload := range []string{
		"B|20|other jank_event_sync: start_ts=0, end_ts=1, jank_frames=2, appid=30",
		"B|20|jank_event_sync_extra: start_ts=0, end_ts=1, jank_frames=2, appid=30",
		"I|20|jank_event_sync: start_ts=0, end_ts=1, jank_frames=2, appid=30",
		"B|20|external_event: start_ts_ns=0, end_ts_ns=1, jank_frames=2, appid=30",
	} {
		ev, ok := ParseLine(1, traceMarkTestLine("writer", 10, 2, payload), newStringInterner())
		if !ok {
			t.Fatalf("raw marker disappeared: %s", payload)
		}
		if ev.PluginFields != nil && ev.JankEvent != nil {
			t.Fatalf("unrecognized grammar gained jank clock authority: %+v", ev)
		}
	}
}
