package tracequery

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestATraceExtendedMarkerStrictGrammar(t *testing.T) {
	valid := []struct {
		payload            string
		action             string
		pid                int
		track, name, value string
	}{
		{"G|237|render-track|phase|42", "G", 237, "render-track", "phase", "42"},
		{"H|237|render-track|42", "H", 237, "render-track", "", "42"},
		{"G|237|render-track|phase|-7", "G", 237, "render-track", "phase", "-7"},
		{"N|237|render-track|checkpoint", "N", 237, "render-track", "checkpoint", ""},
		{"I|237|checkpoint", "I", 237, "", "checkpoint", ""},
	}
	for _, tc := range valid {
		t.Run("valid_"+tc.action+"_"+tc.value, func(t *testing.T) {
			ev, ok := ParseLine(1, traceMarkTestLine("writer", 10, 1, tc.payload), newStringInterner())
			if !ok || ev.Type != EventTraceMark {
				t.Fatalf("extended marker rejected: ok=%t event=%+v", ok, ev)
			}
			if ev.SpanAction != tc.action || ev.SpanPID != tc.pid || traceTrackNameFromEvent(ev) != tc.track || ev.SpanName != tc.name || ev.SpanValue != tc.value {
				t.Fatalf("payload %q parsed as action=%q pid=%d track=%q name=%q value=%q", tc.payload, ev.SpanAction, ev.SpanPID, traceTrackNameFromEvent(ev), ev.SpanName, ev.SpanValue)
			}
			if tc.track != "" && (ev.PluginFields == nil || ev.PluginFields.SpanTrack != tc.track) {
				t.Fatalf("typed span_track side table missing: %+v", ev.PluginFields)
			}
			if tc.track == "" && ev.PluginFields != nil {
				t.Fatalf("track-less marker allocated rare side table: %+v", ev.PluginFields)
			}
			if ev.FieldText != tc.payload {
				t.Fatalf("raw payload provenance changed: got %q want %q", ev.FieldText, tc.payload)
			}
		})
	}

	invalid := []struct {
		payload, action, reason string
	}{
		{"G|bad|track|name|1", "G", "invalid_payload_pid"},
		{"G|2147483648|track|name|1", "G", "invalid_payload_pid"},
		{"G|0|track|name|1", "G", "payload_pid_must_be_positive"},
		{"G|237||name|1", "G", "empty_track_name"},
		{"G|237|track||1", "G", "empty_name"},
		{"G|237|track|name|opaque", "G", "invalid_cookie"},
		{"G|237|track|name|2147483648", "G", "invalid_cookie"},
		{"G|237|track|name|1|extra", "G", "invalid_arity"},
		{"H|237|track|name|1", "H", "invalid_arity"}, // H never carries a name.
		{"H|237|track|", "H", "empty_cookie"},
		{"N|237||name", "N", "empty_track_name"},
		{"N|237|track|", "N", "empty_name"},
		{"I|237|", "I", "empty_name"},
		{"I|237|name|extra", "I", "invalid_arity"},
	}
	for _, tc := range invalid {
		t.Run("invalid_"+tc.action+"_"+tc.reason, func(t *testing.T) {
			parsed := parseTraceMarkValidated(tc.payload)
			if parsed.action != "" || parsed.invalidAction.String() != tc.action || parsed.invalidReason.String() != tc.reason {
				t.Fatalf("payload %q verdict=%+v", tc.payload, parsed)
			}
			ev, ok := ParseLine(1, traceMarkTestLine("writer", 10, 1, tc.payload), newStringInterner())
			if !ok || ev.Type != EventTraceMark || ev.SpanAction != "" || ev.FieldText != tc.payload {
				t.Fatalf("malformed marker must remain raw trace_mark inventory: ok=%t event=%+v", ok, ev)
			}
			action, reason := traceMarkEventInvalidCodes(ev)
			if action.String() != tc.action || reason.String() != tc.reason {
				t.Fatalf("event-level malformed verdict mismatch: action=%s reason=%s event=%+v", action, reason, ev)
			}
		})
	}
}

func TestATraceTrackSpanCarryInAndInstantInventoryStayOutOfThreadSpans(t *testing.T) {
	path := writeTraceMarkIntegrityTrace(t, "extended-markers.systrace",
		traceMarkTestLine("begin-writer", 10, 1.000, "G|237|render-track|VerifyClass|42"),
		traceMarkTestLine("point-writer", 12, 1.001, "N|237|render-track|track-checkpoint"),
		traceMarkTestLine("point-writer", 12, 1.002, "I|237|process-checkpoint"),
		traceMarkTestLine("end-writer", 11, 1.010, "H|237|render-track|42"),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	full := ComputeWindowStats(idx, Query{TimeStart: 1.000, TimeEnd: 1.011})
	if len(full.TraceTrackSpans) != 1 {
		t.Fatalf("expected one track span: %+v caveats=%v", full.TraceTrackSpans, full.Caveats)
	}
	span := full.TraceTrackSpans[0]
	if span.OwnerPID != 237 || span.TrackName != "render-track" || span.Name != "VerifyClass" || span.Cookie != "42" || math.Abs(span.DurationMs-10) > 1e-9 {
		t.Fatalf("track identity/duration mismatch: %+v", span)
	}
	if span.BeginEmitter.PID != 10 || span.EndEmitter.PID != 11 || span.SourcePath != canonicalTraceIndexPath(path) || span.BeginPayload == "" || span.EndPayload == "" {
		t.Fatalf("endpoint provenance missing or emitter forged: %+v", span)
	}
	if len(full.TraceInstants) != 2 || full.TraceInstants[0].Action != "N" || full.TraceInstants[1].Action != "I" {
		t.Fatalf("instant inventory mismatch: %+v", full.TraceInstants)
	}
	if len(full.TraceSpans) != 0 || len(full.TraceMarkCategories) != 0 {
		t.Fatalf("track marker name leaked into thread semantic spans: spans=%+v categories=%+v", full.TraceSpans, full.TraceMarkCategories)
	}
	rank := BuildRootCauseRank(idx, Query{TimeStart: 1.000, TimeEnd: 1.011})
	for _, item := range rank.Items {
		if item.SpanName == "VerifyClass" || item.SemanticClass != "" {
			t.Fatalf("logical track marker leaked into thread/root-cause ranking: %+v", item)
		}
	}

	carry := ComputeWindowStats(idx, Query{TimeStart: 1.005, TimeEnd: 1.012})
	if len(carry.TraceTrackSpans) != 1 {
		t.Fatalf("window-head carry-in pair disappeared: %+v caveats=%v", carry.TraceTrackSpans, carry.Caveats)
	}
	clipped := carry.TraceTrackSpans[0]
	if clipped.StartTs != 1.005 || clipped.EndTs != 1.010 || math.Abs(clipped.DurationMs-5) > 1e-9 || clipped.ActualStartTs != 1.000 || clipped.ActualEndTs != 1.010 || math.Abs(clipped.ActualDurationMs-10) > 1e-9 {
		t.Fatalf("carry-in projection/actual ledger mismatch: %+v", clipped)
	}
	if len(carry.TraceInstants) != 0 {
		t.Fatalf("out-of-window point markers leaked into inventory: %+v", carry.TraceInstants)
	}

	windowed, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 1.005, TimeEnd: 1.012, TimeStartSet: true, TimeEndSet: true,
		TimePaddingBefore: 0.010, TimePaddingAfter: 0.010, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	windowedCarry := ComputeWindowStats(windowed, Query{TimeStart: 1.005, TimeEnd: 1.012})
	if len(windowedCarry.TraceTrackSpans) != 1 || math.Abs(windowedCarry.TraceTrackSpans[0].DurationMs-5) > 1e-9 {
		t.Fatalf("windowed index padding lost G carry-in: %+v caveats=%v", windowedCarry.TraceTrackSpans, windowedCarry.Caveats)
	}
}

func TestATraceTrackDuplicateKeyFailsClosedButIndependentKeysPair(t *testing.T) {
	path := writeTraceMarkIntegrityTrace(t, "track-concurrency.systrace",
		traceMarkTestLine("w1", 10, 2.000, "G|237|same-track|first|1"),
		traceMarkTestLine("w2", 11, 2.001, "G|237|same-track|second|1"),
		traceMarkTestLine("w3", 12, 2.002, "G|237|other-track|independent-track|1"),
		traceMarkTestLine("w4", 13, 2.003, "G|237|same-track|independent-cookie|2"),
		traceMarkTestLine("w5", 14, 2.004, "H|237|same-track|1"),
		traceMarkTestLine("w6", 15, 2.005, "H|237|other-track|1"),
		traceMarkTestLine("w7", 16, 2.006, "H|237|same-track|2"),
		traceMarkTestLine("w8", 17, 2.007, "H|237|same-track|1"),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 1.999, TimeEnd: 2.008})
	if len(stats.TraceTrackSpans) != 2 {
		t.Fatalf("independent track/cookie keys must pair while duplicate key is withheld: %+v caveats=%v", stats.TraceTrackSpans, stats.Caveats)
	}
	for _, span := range stats.TraceTrackSpans {
		if span.Name == "first" || span.Name == "second" {
			t.Fatalf("ambiguous duplicate key minted a duration: %+v", span)
		}
	}
	if !caveatsContain(stats.Caveats, "trace_track_duplicate_key_fail_closed=true") {
		t.Fatalf("duplicate-key suppression was silent: %v", stats.Caveats)
	}
}

func TestATraceTrackPairingRespectsPhysicalSourceGenerationAndTimeOrder(t *testing.T) {
	t.Run("physical_source", func(t *testing.T) {
		idx := durationBundleIndex([]Event{
			{Line: 1, Ts: 3.000, Type: EventTraceMark, PID: 10, SpanAction: "G", SpanPID: 237, SpanName: "work", SpanValue: "7", FieldText: "G|237|track|work|7"},
			{Line: 101, Ts: 3.010, Type: EventTraceMark, PID: 11, SpanAction: "H", SpanPID: 237, SpanValue: "7", FieldText: "H|237|track|7"},
		})
		stats := ComputeWindowStats(idx, Query{TimeStart: 2.999, TimeEnd: 3.011})
		if len(stats.TraceTrackSpans) != 0 {
			t.Fatalf("track endpoints crossed physical artifacts: %+v", stats.TraceTrackSpans)
		}
	})

	t.Run("payload_owner_generation", func(t *testing.T) {
		path := writeTraceMarkIntegrityTrace(t, "track-generation.systrace",
			traceMarkTestLine("writer", 10, 4.000, "G|237|track|old-generation|7"),
			` creator-20 (20) [000] .... 4.005000: sched_wakeup_new: comm=reused pid=237 prio=120 target_cpu=000`,
			traceMarkTestLine("writer", 11, 4.010, "H|237|track|7"),
		)
		idx, err := BuildIndex(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		stats := ComputeWindowStats(idx, Query{TimeStart: 3.999, TimeEnd: 4.011})
		if len(stats.TraceTrackSpans) != 0 {
			t.Fatalf("track endpoints crossed payload-owner generation: %+v", stats.TraceTrackSpans)
		}
	})

	t.Run("physical_time_regression", func(t *testing.T) {
		path := writeTraceMarkIntegrityTrace(t, "track-time-regression.systrace",
			traceMarkTestLine("writer", 10, 5.010, "G|237|track|work|7"),
			traceMarkTestLine("writer", 11, 5.000, "H|237|track|7"),
		)
		idx, err := BuildIndex(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		stats := ComputeWindowStats(idx, Query{TimeStart: 4.999, TimeEnd: 5.011})
		if len(stats.TraceTrackSpans) != 0 || !caveatsContain(stats.Caveats, "family=trace_track_span") {
			t.Fatalf("time-regressed track lane did not fail closed: spans=%+v caveats=%v", stats.TraceTrackSpans, stats.Caveats)
		}
	})
}

func TestMalformedTrackEndpointSuppressesOnlyTrackDurationFace(t *testing.T) {
	path := writeTraceMarkIntegrityTrace(t, "track-malformed.systrace",
		traceMarkTestLine("writer", 10, 6.000, "G|237|track|bad-cookie|opaque"),
		traceMarkTestLine("writer", 10, 6.001, "G|237|other|valid|9"),
		traceMarkTestLine("writer", 11, 6.002, "H|237|other|9"),
		traceMarkTestLine("writer", 12, 6.0025, "I|237|"),
		traceMarkTestLine("writer", 12, 6.003, "I|237|still-visible"),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	stats := ComputeWindowStats(idx, Query{TimeStart: 5.999, TimeEnd: 6.004})
	if len(stats.TraceTrackSpans) != 0 || !caveatsContain(stats.Caveats, "trace_track_pairing_fail_closed=true") {
		t.Fatalf("malformed G endpoint did not close track-duration face: spans=%+v caveats=%v", stats.TraceTrackSpans, stats.Caveats)
	}
	if len(stats.TraceInstants) != 1 || stats.TraceInstants[0].Name != "still-visible" {
		t.Fatalf("malformed duration endpoint incorrectly erased instant inventory: %+v", stats.TraceInstants)
	}
	if !caveatsContain(stats.Caveats, "actions=[G I]") {
		t.Fatalf("malformed I row was omitted without typed integrity disclosure: %v", stats.Caveats)
	}
	for _, caveat := range stats.Caveats {
		if strings.Contains(caveat, "trace_mark_span_pairing_fail_closed=true") {
			t.Fatalf("G/H malformation poisoned classic B/E/S/F spans: %v", stats.Caveats)
		}
	}
}

func TestATraceExtendedProvenanceFailuresAreFaceScoped(t *testing.T) {
	t.Run("unresolved_instant_keeps_track_spans", func(t *testing.T) {
		idx := durationBundleIndex([]Event{
			{Line: 1, Ts: 7.000, Type: EventTraceMark, PID: 10, SpanAction: "G", SpanPID: 321, SpanName: "work", SpanValue: "7", FieldText: "G|321|track|work|7"},
			{Line: 2, Ts: 7.010, Type: EventTraceMark, PID: 11, SpanAction: "H", SpanPID: 321, SpanValue: "7", FieldText: "H|321|track|7"},
			{Line: 999, Ts: 7.005, Type: EventTraceMark, PID: 12, SpanAction: "I", SpanPID: 321, SpanName: "point", FieldText: "I|321|point"},
		})
		stats := ComputeWindowStats(idx, Query{TimeStart: 6.999, TimeEnd: 7.011})
		if len(stats.TraceTrackSpans) != 1 || len(stats.TraceInstants) != 0 {
			t.Fatalf("unresolved instant crossed inventory faces: spans=%+v instants=%+v caveats=%v", stats.TraceTrackSpans, stats.TraceInstants, stats.Caveats)
		}
		if !caveatsContain(stats.Caveats, "trace_instant_provenance_unresolved=true") {
			t.Fatalf("unresolved instant omission was silent: %v", stats.Caveats)
		}
	})

	t.Run("unresolved_track_keeps_instants", func(t *testing.T) {
		idx := durationBundleIndex([]Event{
			{Line: 1, Ts: 8.000, Type: EventTraceMark, PID: 12, SpanAction: "N", SpanPID: 321, SpanName: "point", FieldText: "N|321|track|point"},
			{Line: 999, Ts: 8.001, Type: EventTraceMark, PID: 10, SpanAction: "G", SpanPID: 321, SpanName: "work", SpanValue: "7", FieldText: "G|321|track|work|7"},
		})
		stats := ComputeWindowStats(idx, Query{TimeStart: 7.999, TimeEnd: 8.002})
		if len(stats.TraceTrackSpans) != 0 || len(stats.TraceInstants) != 1 {
			t.Fatalf("unresolved track endpoint crossed inventory faces: spans=%+v instants=%+v caveats=%v", stats.TraceTrackSpans, stats.TraceInstants, stats.Caveats)
		}
		if !caveatsContain(stats.Caveats, "trace_track_provenance_unresolved=true") {
			t.Fatalf("unresolved track omission was silent: %v", stats.Caveats)
		}
	})
}
