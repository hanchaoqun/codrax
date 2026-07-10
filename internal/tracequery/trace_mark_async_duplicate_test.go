package tracequery

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func asyncDuplicateEvent(line int, ts float64, action string, emitterPID, payloadPID int, name, cookie string) Event {
	return Event{
		Line:       line,
		Ts:         ts,
		Type:       EventTraceMark,
		PID:        emitterPID,
		Comm:       "writer",
		SpanAction: action,
		SpanPID:    payloadPID,
		SpanName:   name,
		SpanValue:  cookie,
		FieldText:  strings.Join([]string{action, strconv.Itoa(payloadPID), name, cookie}, "|"),
	}
}

func asyncDuplicateIndex(events []Event) *Index {
	idx := &Index{
		Path:           "/trace/async-duplicate.systrace",
		Events:         events,
		TimestampOrder: TraceTimestampOrderMonotonic,
	}
	for _, ev := range events {
		if idx.FirstTs == 0 || ev.Ts < idx.FirstTs {
			idx.FirstTs = ev.Ts
		}
		if ev.Ts > idx.LastTs {
			idx.LastTs = ev.Ts
		}
		if ev.Line > idx.LineCount {
			idx.LineCount = ev.Line
		}
	}
	return idx
}

func asyncSpanStartLines(spans []TraceSpanSummary) map[int]bool {
	out := map[int]bool{}
	for _, span := range spans {
		if span.Kind == "async" {
			out[span.StartLine] = true
		}
	}
	return out
}

func asyncPairingCaveats(caveats []string) []string {
	var out []string
	for _, caveat := range caveats {
		if strings.Contains(caveat, "trace_mark_async_") {
			out = append(out, caveat)
		}
	}
	return out
}

func TestTraceMarkAsyncDuplicateCohortFailsClosedAndRecovers(t *testing.T) {
	events := []Event{
		// Same exact wire key: this four-endpoint cohort is unpairable.
		asyncDuplicateEvent(1, 1.000, "S", 100, 10, "VerifyClass", "7"),
		asyncDuplicateEvent(2, 1.001, "S", 101, 10, "VerifyClass", "7"),
		// Independent cookie, name, and payload owner remain pairable while the
		// ambiguous key is still open.
		asyncDuplicateEvent(3, 1.002, "S", 102, 10, "VerifyClass", "8"),
		asyncDuplicateEvent(4, 1.003, "F", 103, 10, "VerifyClass", "8"),
		asyncDuplicateEvent(5, 1.004, "S", 104, 10, "ShaderCompile", "7"),
		asyncDuplicateEvent(6, 1.005, "F", 105, 10, "ShaderCompile", "7"),
		asyncDuplicateEvent(7, 1.006, "S", 106, 11, "VerifyClass", "7"),
		asyncDuplicateEvent(8, 1.007, "F", 107, 11, "VerifyClass", "7"),
		asyncDuplicateEvent(9, 1.008, "F", 108, 10, "VerifyClass", "7"),
		asyncDuplicateEvent(10, 1.009, "F", 109, 10, "VerifyClass", "7"),
		// Depth returned to zero above, so this new cohort is healthy.
		asyncDuplicateEvent(11, 1.010, "S", 110, 10, "VerifyClass", "7"),
		asyncDuplicateEvent(12, 1.012, "F", 111, 10, "VerifyClass", "7"),
	}
	idx := asyncDuplicateIndex(events)
	q := Query{TimeStart: .999, TimeEnd: 1.020, TimeStartSet: true, TimeEndSet: true}

	stats := ComputeWindowStats(idx, q)
	wantLines := map[int]bool{3: true, 5: true, 7: true, 11: true}
	if got := asyncSpanStartLines(stats.TraceSpans); !reflect.DeepEqual(got, wantLines) {
		t.Fatalf("window_stats guessed a duplicate-key pair or lost an independent/recovered pair: got=%v spans=%+v", got, stats.TraceSpans)
	}
	if !containsSubstring(stats.Caveats, "trace_mark_async_duplicate_key_fail_closed=true; ambiguous_cohorts=1 ambiguous_starts=2") {
		t.Fatalf("window_stats did not disclose the withheld cohort: %+v", stats.Caveats)
	}
	if containsSubstring(stats.Caveats, "trace_mark_async_pairing_incomplete=true") {
		t.Fatalf("balanced S/S/F/F must discharge depth without orphan/incomplete accounting: %+v", stats.Caveats)
	}

	spans, caveats := FindSpanWindows(idx, q, 16)
	if got := asyncSpanStartLines(spans); !reflect.DeepEqual(got, wantLines) {
		t.Fatalf("span_window drifted from window_stats async pairing: got=%v spans=%+v", got, spans)
	}
	if !reflect.DeepEqual(asyncPairingCaveats(caveats), asyncPairingCaveats(stats.Caveats)) {
		t.Fatalf("window_stats/span_window async caveat drift:\nstats=%v\nspan=%v", asyncPairingCaveats(stats.Caveats), asyncPairingCaveats(caveats))
	}
}

func TestTraceMarkAsyncDuplicateNeverMintsRootRankDuration(t *testing.T) {
	idx := asyncDuplicateIndex([]Event{
		asyncDuplicateEvent(1, 2.000, "S", 10, 42, "VerifyClass", "same"),
		asyncDuplicateEvent(2, 2.001, "S", 11, 42, "VerifyClass", "same"),
		asyncDuplicateEvent(3, 2.010, "F", 12, 42, "VerifyClass", "same"),
		asyncDuplicateEvent(4, 2.020, "F", 13, 42, "VerifyClass", "same"),
	})
	res := Run(idx, Query{
		View: "root_cause_rank", TimeStart: 1.9, TimeEnd: 2.1,
		TimeStartSet: true, TimeEndSet: true, Limit: 16,
	})
	if res.WindowStats == nil || len(res.WindowStats.TraceSpans) != 0 {
		t.Fatalf("ambiguous async cohort reached root-rank input: %+v", res.WindowStats)
	}
	if res.RootCauseRank == nil {
		t.Fatal("root_cause_rank result missing")
	}
	for _, item := range res.RootCauseRank.Items {
		if item.SpanName == "VerifyClass" || item.SemanticClass == "class_verification" {
			t.Fatalf("ambiguous async duration minted a root-rank contender: %+v", item)
		}
	}
	if !containsSubstring(res.WindowStats.Caveats, "trace_mark_async_duplicate_key_fail_closed=true") {
		t.Fatalf("root-rank input omitted duplicate cohort disclosure: %+v", res.WindowStats.Caveats)
	}
}

func TestTraceMarkAsyncLifecycleGenerationAndSourceIsolation(t *testing.T) {
	t.Run("payload_owner_generation", func(t *testing.T) {
		idx := asyncDuplicateIndex([]Event{
			asyncDuplicateEvent(1, 3.000, "S", 10, 42, "generation-work", "1"),
			asyncDuplicateEvent(2, 3.001, "S", 10, 43, "independent-owner", "1"),
			{Line: 3, Ts: 3.010, Type: EventSchedWakeup, Name: "sched_wakeup_new", PID: 7, WakeePID: 42, WakeeComm: "new-owner"},
			asyncDuplicateEvent(4, 3.020, "F", 11, 42, "generation-work", "1"),
			asyncDuplicateEvent(5, 3.021, "F", 11, 43, "independent-owner", "1"),
			asyncDuplicateEvent(6, 3.030, "S", 12, 42, "generation-work", "1"),
			asyncDuplicateEvent(7, 3.040, "F", 13, 42, "generation-work", "1"),
		})
		q := Query{TimeStart: 2.9, TimeEnd: 3.1, TimeStartSet: true, TimeEndSet: true}
		stats := ComputeWindowStats(idx, q)
		if got, want := asyncSpanStartLines(stats.TraceSpans), map[int]bool{2: true, 6: true}; !reflect.DeepEqual(got, want) {
			t.Fatalf("lifecycle boundary crossed generation or polluted another payload owner: got=%v spans=%+v", got, stats.TraceSpans)
		}
		for _, want := range []string{"incomplete_begins=1", "orphan_ends=1", "lifecycle_cuts=1"} {
			if !containsSubstring(stats.Caveats, want) {
				t.Fatalf("lifecycle accounting missing %q: %+v", want, stats.Caveats)
			}
		}
		spans, caveats := FindSpanWindows(idx, q, 16)
		if got, want := asyncSpanStartLines(spans), map[int]bool{2: true, 6: true}; !reflect.DeepEqual(got, want) {
			t.Fatalf("span_window crossed payload-owner generation: got=%v spans=%+v", got, spans)
		}
		if !reflect.DeepEqual(asyncPairingCaveats(caveats), asyncPairingCaveats(stats.Caveats)) {
			t.Fatalf("generation caveat drift: stats=%v span=%v", asyncPairingCaveats(stats.Caveats), asyncPairingCaveats(caveats))
		}
	})

	t.Run("physical_source", func(t *testing.T) {
		idx := durationBundleIndex([]Event{
			asyncDuplicateEvent(1, 4.000, "S", 10, 42, "same-wire", "9"),
			asyncDuplicateEvent(101, 4.001, "S", 10, 42, "same-wire", "9"),
			asyncDuplicateEvent(102, 4.002, "F", 11, 42, "same-wire", "9"),
			asyncDuplicateEvent(2, 4.003, "F", 11, 42, "same-wire", "9"),
		})
		q := Query{TimeStart: 3.9, TimeEnd: 4.1, TimeStartSet: true, TimeEndSet: true}
		stats := ComputeWindowStats(idx, q)
		if got, want := asyncSpanStartLines(stats.TraceSpans), map[int]bool{1: true, 101: true}; !reflect.DeepEqual(got, want) {
			t.Fatalf("identical wire keys in independent artifacts polluted each other: got=%v spans=%+v", got, stats.TraceSpans)
		}
		if containsSubstring(stats.Caveats, "trace_mark_async_duplicate_key_fail_closed=true") || containsSubstring(stats.Caveats, "trace_mark_async_pairing_incomplete=true") {
			t.Fatalf("independent physical sources were treated as one lane: %+v", stats.Caveats)
		}
	})
}

func TestTraceMarkAsyncWindowBoundaryCompactionAndIncompleteAccounting(t *testing.T) {
	idx := asyncDuplicateIndex([]Event{
		// Ambiguous carry-in cohort overlaps the selected window and must not
		// acquire either of the two long LIFO durations.
		asyncDuplicateEvent(1, 4.800, "S", 10, 50, "work", "dup"),
		asyncDuplicateEvent(2, 4.900, "S", 11, 50, "work", "dup"),
		asyncDuplicateEvent(3, 4.950, "S", 12, 50, "work", "cross"),
		asyncDuplicateEvent(4, 5.020, "F", 13, 50, "work", "dup"),
		asyncDuplicateEvent(5, 5.030, "F", 14, 50, "work", "dup"),
		asyncDuplicateEvent(6, 5.050, "F", 15, 50, "work", "cross"),
		asyncDuplicateEvent(7, 5.060, "S", 16, 50, "work", "two"),
		asyncDuplicateEvent(8, 5.080, "F", 17, 50, "work", "two"),
		asyncDuplicateEvent(9, 5.090, "S", 18, 50, "work", "three"),
		asyncDuplicateEvent(10, 5.100, "F", 19, 50, "work", "three"),
		asyncDuplicateEvent(11, 5.105, "S", 20, 50, "work", "open"),
		asyncDuplicateEvent(12, 5.110, "F", 21, 50, "work", "orphan"),
	})
	q := Query{SpanName: "work", TimeStart: 5.000, TimeEnd: 5.120, TimeStartSet: true, TimeEndSet: true}
	stats := ComputeWindowStats(idx, q)
	if got, want := asyncSpanStartLines(stats.TraceSpans), map[int]bool{3: true, 7: true, 9: true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("window boundary async pairing mismatch: got=%v spans=%+v", got, stats.TraceSpans)
	}
	for _, span := range stats.TraceSpans {
		if span.StartLine == 3 && (!near(span.DurationMs, 50, .001) || !near(span.ActualDurationMs, 100, .001)) {
			t.Fatalf("valid carry-in span was not clipped honestly: %+v", span)
		}
	}
	for _, want := range []string{"ambiguous_cohorts=1", "incomplete_begins=1", "orphan_ends=1"} {
		if !containsSubstring(stats.Caveats, want) {
			t.Fatalf("window accounting missing %q: %+v", want, stats.Caveats)
		}
	}

	spans, caveats, compaction := findSpanWindowsCompacted(idx, q, 2)
	if len(spans) != 2 || compaction == nil || compaction.Total != 3 || compaction.Emitted != 2 {
		t.Fatalf("ambiguous/incomplete cohorts distorted span compaction: spans=%+v compaction=%+v", spans, compaction)
	}
	if !reflect.DeepEqual(asyncPairingCaveats(caveats), asyncPairingCaveats(stats.Caveats)) {
		t.Fatalf("boundary caveat drift: stats=%v span=%v", asyncPairingCaveats(stats.Caveats), asyncPairingCaveats(caveats))
	}
}

func TestTraceMarkAsyncStreamScanAndIndexedPairingParity(t *testing.T) {
	path := writeTraceMarkIntegrityTrace(t, "async-duplicate-stream.systrace",
		traceMarkTestLine("w1", 10, 6.000, "S|42|VerifyClass|7"),
		traceMarkTestLine("w2", 11, 6.001, "S|42|VerifyClass|7"),
		traceMarkTestLine("w3", 12, 6.010, "F|42|VerifyClass|7"),
		traceMarkTestLine("w4", 13, 6.020, "F|42|VerifyClass|7"),
		traceMarkTestLine("w5", 14, 6.030, "S|42|VerifyClass|7"),
		traceMarkTestLine("w6", 15, 6.040, "F|42|VerifyClass|7"),
	)
	indexed, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var streamedEvents []Event
	streamed, err := StreamScan(context.Background(), path, TraceFlavorAuto, func(ev Event) bool {
		streamedEvents = append(streamedEvents, ev)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	// StreamScan intentionally returns a metadata shell. Attach the callback
	// sequence only in this package test to exercise the same pairing consumer
	// over the streaming parser's event delivery.
	streamed.Events = streamedEvents
	streamed.TimestampOrder = TraceTimestampOrderMonotonic

	q := Query{TimeStart: 5.9, TimeEnd: 6.1, TimeStartSet: true, TimeEndSet: true}
	indexedStats := ComputeWindowStats(indexed, q)
	streamedStats := ComputeWindowStats(streamed, q)
	if !reflect.DeepEqual(indexedStats.TraceSpans, streamedStats.TraceSpans) {
		t.Fatalf("stream/index async spans drift:\nindexed=%+v\nstream=%+v", indexedStats.TraceSpans, streamedStats.TraceSpans)
	}
	if !reflect.DeepEqual(asyncPairingCaveats(indexedStats.Caveats), asyncPairingCaveats(streamedStats.Caveats)) {
		t.Fatalf("stream/index async caveats drift:\nindexed=%v\nstream=%v", asyncPairingCaveats(indexedStats.Caveats), asyncPairingCaveats(streamedStats.Caveats))
	}
	indexedSpans, indexedCaveats := FindSpanWindows(indexed, q, 16)
	streamedSpans, streamedCaveats := FindSpanWindows(streamed, q, 16)
	if !reflect.DeepEqual(indexedSpans, streamedSpans) || !reflect.DeepEqual(asyncPairingCaveats(indexedCaveats), asyncPairingCaveats(streamedCaveats)) {
		t.Fatalf("stream/index span_window drift:\nindexed=%+v %v\nstream=%+v %v", indexedSpans, asyncPairingCaveats(indexedCaveats), streamedSpans, asyncPairingCaveats(streamedCaveats))
	}
}
