package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const traceMarkActionFilterFixture = ` app-10 (10) [000] .... 1.000000: tracing_mark_write: B|10|sync-work
 app-10 (10) [000] .... 1.001000: tracing_mark_write: E|10
 app-10 (10) [000] .... 1.002000: tracing_mark_write: C|10|counter|1|S|10|
 app-10 (10) [000] .... 1.003000: tracing_mark_write: S|10|async-work|7
 app-10 (10) [000] .... 1.004000: tracing_mark_write: F|10|async-work|7
 app-10 (10) [000] .... 1.005000: tracing_mark_write: G|10|track|track-work|9
 app-10 (10) [000] .... 1.006000: tracing_mark_write: H|10|track|9
 app-10 (10) [000] .... 1.007000: tracing_mark_write: N|10|track|track-point
 app-10 (10) [000] .... 1.008000: tracing_mark_write: I|10|process-point
 app-10 (10) [000] .... 1.009000: tracing_mark_write: S|bad|malformed|11
 app-10 (10) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=10 prev_prio=20 prev_state=R ==> next_comm=idle/0 next_pid=0 next_prio=120
`

func writeTraceMarkActionFilterFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "actions.systrace")
	if err := os.WriteFile(path, []byte(traceMarkActionFilterFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTraceMarkActionFilterIndexedStreamingParityAndIntegrity(t *testing.T) {
	path := writeTraceMarkActionFilterFixture(t)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{
		View:             FallbackViewEventSearch,
		TimeStart:        1,
		TimeEnd:          2,
		TimeStartSet:     true,
		TimeEndSet:       true,
		TraceMarkActions: []TraceMarkAction{TraceMarkActionAsyncBegin},
		Limit:            40,
	}
	indexed := Run(idx, q)
	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	indexedLines, streamedLines := eventViewLines(indexed.Events), eventViewLines(streamed.Events)
	if !reflect.DeepEqual(indexedLines, streamedLines) || !reflect.DeepEqual(indexedLines, []int{4}) {
		t.Fatalf("action-filter parity indexed=%v streamed=%v; want only valid S row line 4", indexedLines, streamedLines)
	}
	for _, result := range []Result{indexed, streamed} {
		if len(result.Events) != 1 || result.Events[0].SpanAction != "S" || result.Events[0].SpanName != "async-work" {
			t.Fatalf("typed action result admitted a raw-prefix impostor or malformed row: %+v", result.Events)
		}
		if !containsStringFragment(result.Caveats, "trace_mark_integrity_degraded=true") ||
			!containsStringFragment(result.Caveats, "invalid_payload_pid") {
			t.Fatalf("action filter hid malformed marker integrity witness: %v", result.Caveats)
		}
	}
}

func TestTraceMarkActionFilterIndexedStreamingAccountingParity(t *testing.T) {
	path := writeTraceMarkActionFilterFixture(t)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{
		View:             FallbackViewEventSearch,
		TraceMarkActions: []TraceMarkAction{TraceMarkActionAsyncBegin, TraceMarkActionAsyncEnd},
		Limit:            1,
	}
	indexed := Run(idx, q)
	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	for name, result := range map[string]Result{"indexed": indexed, "streamed": streamed} {
		if lines := eventViewLines(result.Events); !reflect.DeepEqual(lines, []int{4}) {
			t.Fatalf("%s earliest emitted roster = %v, want [4]", name, lines)
		}
		if len(result.Compactions) != 1 {
			t.Fatalf("%s compactions = %+v, want one exact accounting row", name, result.Compactions)
		}
		compaction := result.Compactions[0]
		if compaction.Total != 2 || compaction.Emitted != 1 || compaction.LastEmittedLine != 4 {
			t.Fatalf("%s accounting = %+v, want matched=2 emitted=1 last line=4", name, compaction)
		}
	}
}

func TestTraceMarkActionFilterDirectEngineBoundariesFailClosed(t *testing.T) {
	path := writeTraceMarkActionFilterFixture(t)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	invalid := Query{
		View:             "window_stats",
		TraceMarkActions: []TraceMarkAction{TraceMarkActionAsyncBegin},
		Limit:            40,
	}
	result := Run(idx, invalid)
	if len(result.Events) != 0 || result.WindowStats != nil || !containsStringFragment(result.Caveats, "trace_mark_action_filter_invalid=true") {
		t.Fatalf("Run must reject before evaluating any view: %+v", result)
	}
	if result.SourcePath != idx.Path || result.LineCount != idx.LineCount || result.EventCount != len(idx.Events) {
		t.Fatalf("rejected Run must retain source metadata: %+v", result)
	}
	if events := EventSearch(idx, invalid); len(events) != 0 {
		t.Fatalf("direct EventSearch bypass admitted invalid action filter: %+v", events)
	}
	if _, err := StreamEventSearch(context.Background(), path, invalid); err == nil || !strings.Contains(err.Error(), "only valid for view=event_search") {
		t.Fatalf("stream boundary must reject wrong-view action filter, got %v", err)
	}

	duplicate := Query{View: FallbackViewEventSearch, TraceMarkActions: []TraceMarkAction{"S", "S"}, Limit: 40}
	result = Run(idx, duplicate)
	if len(result.Events) != 0 || !containsStringFragment(result.Caveats, "duplicate trace_mark action") {
		t.Fatalf("duplicate action must fail closed with typed caveat: %+v", result)
	}
}

func TestTraceMarkActionFilterClosedSetAndConjunction(t *testing.T) {
	path := writeTraceMarkActionFilterFixture(t)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	actions := []TraceMarkAction{
		TraceMarkActionBegin, TraceMarkActionEnd, TraceMarkActionCounter,
		TraceMarkActionAsyncBegin, TraceMarkActionAsyncEnd,
		TraceMarkActionTrackBegin, TraceMarkActionTrackEnd,
		TraceMarkActionTrackInstant, TraceMarkActionInstant,
	}
	for i, action := range actions {
		got := EventSearch(idx, Query{TraceMarkActions: []TraceMarkAction{action}, Limit: 40})
		if len(got) != 1 || got[0].SpanAction != string(action) || got[0].Line != i+1 {
			t.Errorf("action %s => %+v, want line=%d exact typed action", action, got, i+1)
		}
	}
	got := EventSearch(idx, Query{
		EventTypes:       []EventType{EventTraceMark},
		TraceMarkActions: []TraceMarkAction{TraceMarkActionAsyncBegin},
		Pattern:          "async-work",
		Limit:            40,
	})
	if len(got) != 1 || got[0].Line != 4 {
		t.Fatalf("event type + action + pattern must be an intersection, got %+v", got)
	}
	got = EventSearch(idx, Query{TraceMarkActions: []TraceMarkAction{TraceMarkActionAsyncBegin}, Pattern: "counter", Limit: 40})
	if len(got) != 0 {
		t.Fatalf("pattern matched a C payload but exact S action filter must reject it: %+v", got)
	}
}

func TestValidateTraceMarkActionFilterAndCPUGlobalRegistry(t *testing.T) {
	valid := []TraceMarkAction{TraceMarkActionBegin, TraceMarkActionEnd, TraceMarkActionCounter, TraceMarkActionAsyncBegin, TraceMarkActionAsyncEnd, TraceMarkActionTrackBegin, TraceMarkActionTrackEnd, TraceMarkActionTrackInstant, TraceMarkActionInstant}
	if got := TraceMarkActionNames(); !reflect.DeepEqual(got, []string{"B", "E", "C", "S", "F", "G", "H", "N", "I"}) {
		t.Fatalf("canonical action registry = %v", got)
	}
	if err := ValidateTraceMarkActionFilter("event_search", nil, valid); err != nil {
		t.Fatalf("canonical actions rejected: %v", err)
	}
	for name, err := range map[string]error{
		"unknown":      ValidateTraceMarkActionFilter("event_search", nil, []TraceMarkAction{"X"}),
		"duplicate":    ValidateTraceMarkActionFilter("event_search", nil, []TraceMarkAction{"S", "S"}),
		"wrong view":   ValidateTraceMarkActionFilter("window_stats", nil, []TraceMarkAction{"S"}),
		"mixed types":  ValidateTraceMarkActionFilter("event_search", []EventType{EventTraceMark, EventSchedSwitch}, []TraceMarkAction{"S"}),
		"foreign type": ValidateTraceMarkActionFilter("event_search", []EventType{EventUnknown}, []TraceMarkAction{"S"}),
	} {
		if err == nil {
			t.Errorf("%s must fail loud", name)
		}
	}
	if err := ValidateTraceMarkActionFilter("event_search", []EventType{EventTraceMark}, []TraceMarkAction{"S"}); err != nil {
		t.Fatalf("exact trace_mark type rejected: %v", err)
	}
	global := CPUGlobalEventSearchTypes([]EventType{EventSchedSwitch, EventCPUIdle, EventCPUFrequency, EventCPUFrequencyLimit, EventClockSetRate, EventCPUIdle})
	if !reflect.DeepEqual(global, []EventType{EventClockSetRate, EventCPUFrequency, EventCPUFrequencyLimit, EventCPUIdle}) {
		t.Fatalf("CPU-global registry = %v", global)
	}
}

func eventViewLines(events []EventView) []int {
	out := make([]int, len(events))
	for i := range events {
		out[i] = events[i].Line
	}
	return out
}

func containsStringFragment(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}
