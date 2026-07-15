package tracequery

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestTraceMarkWireScalarsAreExact(t *testing.T) {
	invalid := []struct {
		payload string
		action  string
		reason  traceMarkInvalidReason
	}{
		{payload: "B| 42|work", action: "B", reason: traceMarkReasonInvalidPayloadPID},
		{payload: "B|42 |work", action: "B", reason: traceMarkReasonInvalidPayloadPID},
		{payload: "E|42| ", action: "E", reason: traceMarkReasonInvalidEndTag},
		{payload: "B|42|work|D01 ", action: "B", reason: traceMarkReasonInvalidArity},
		{payload: "S|42|work|cookie|I42 ", action: "S", reason: traceMarkReasonInvalidArity},
		{payload: "G| 42|track|work|7", action: "G", reason: traceMarkReasonInvalidPayloadPID},
		{payload: "G|42|track|work|7 ", action: "G", reason: traceMarkReasonInvalidCookie},
		{payload: "H|42|track| 7", action: "H", reason: traceMarkReasonInvalidCookie},
		{payload: "N|42 |track|point", action: "N", reason: traceMarkReasonInvalidPayloadPID},
		{payload: "I| 42|point", action: "I", reason: traceMarkReasonInvalidPayloadPID},
		// An address-prefixed action-present row is still the native strict
		// lane; only an action-lost literal 0x/0X address arm may use carved
		// recovery.
		{payload: "0xabc: B| 42|work", action: "B", reason: traceMarkReasonInvalidPayloadPID},
	}
	for _, tc := range invalid {
		t.Run(strings.NewReplacer("|", "_", " ", "space").Replace(tc.payload), func(t *testing.T) {
			ev, ok := ParseLine(1, traceMarkTestLine("writer", 9, 1, tc.payload), newStringInterner())
			if !ok || ev.Type != EventTraceMark {
				t.Fatalf("malformed exact-prefix marker must remain typed inventory: ok=%v event=%+v", ok, ev)
			}
			action, reason := traceMarkEventInvalidCodes(ev)
			if ev.SpanAction != "" || action.String() != tc.action || reason != tc.reason {
				t.Fatalf("payload %q minted or reported the wrong verdict: event=%+v action=%q reason=%q", tc.payload, ev, action, reason)
			}
		})
	}

	for _, payload := range []string{"E ", "E\r", "B |42|work", "\u00a0B|42|work"} {
		ev, ok := ParseLine(1, traceMarkTestLine("writer", 9, 1, payload), newStringInterner())
		if !ok {
			t.Fatalf("non-marker print row %q should remain searchable inventory", payload)
		}
		if ev.Type == EventTraceMark || ev.SpanAction != "" {
			t.Fatalf("inexact action scalar %q minted a marker: %+v", payload, ev)
		}
	}
}

func TestTraceMarkOpaqueRightEdgeAndCarvedLane(t *testing.T) {
	tests := []struct {
		payload, action, name, track, value string
	}{
		{payload: "B|42|phase ", action: "B", name: "phase "},
		{payload: "B|42|phase |I42", action: "B", name: "phase ", value: "I42"},
		{payload: "S|42|async |cookie ", action: "S", name: "async ", value: "cookie "},
		{payload: "F|42|async |cookie ", action: "F", name: "async ", value: "cookie "},
		{payload: "S|42|async |cookie |I42", action: "S", name: "async ", value: "cookie "},
		{payload: "F|42|async |cookie |I42", action: "F", name: "async ", value: "cookie "},
		{payload: "G|42|track |work |7", action: "G", name: "work ", track: "track ", value: "7"},
		{payload: "H|42|track |7", action: "H", track: "track ", value: "7"},
		{payload: "N|42|track |point ", action: "N", name: "point ", track: "track "},
		{payload: "I|42|point ", action: "I", name: "point "},
		{payload: "0xabc: S|42|addressed |cookie ", action: "S", name: "addressed ", value: "cookie "},
		// The action-lost literal 0x-address arm is a separately disclosed
		// compatibility lane (0x0 is this test's representative):
		// inferred scalars are canonicalized, but the surviving opaque name edge
		// must remain byte-exact.
		{payload: "0x0:  42|carved ", action: "B", name: "carved "},
	}
	for _, tc := range tests {
		ev, ok := ParseLine(1, traceMarkTestLine("writer", 9, 1, tc.payload), newStringInterner())
		if !ok || ev.Type != EventTraceMark || ev.SpanAction != tc.action || ev.SpanName != tc.name || ev.SpanValue != tc.value || traceTrackNameFromEvent(ev) != tc.track {
			t.Fatalf("payload %q lost its exact typed edge: event=%+v track=%q", tc.payload, ev, traceTrackNameFromEvent(ev))
		}
		if ev.FieldText != tc.payload {
			t.Fatalf("payload %q lost event_search provenance: FieldText=%q", tc.payload, ev.FieldText)
		}
	}
}

func TestTraceMarkPairingUsesFullRightEdgeIdentity(t *testing.T) {
	longPrefix := strings.Repeat("x", 320)
	path := writeTraceMarkIntegrityTrace(t, "right-edge-pairing.systrace",
		traceMarkTestLine("writer", 9, 1.000, "S|42|name|cookie"),
		traceMarkTestLine("writer", 9, 1.001, "S|42|name |cookie"),
		traceMarkTestLine("writer", 9, 1.002, "F|42|name|cookie"),
		traceMarkTestLine("writer", 9, 1.003, "F|42|name |cookie"),
		traceMarkTestLine("writer", 9, 1.004, "S|42|cookie-edge|value"),
		traceMarkTestLine("writer", 9, 1.005, "S|42|cookie-edge|value "),
		traceMarkTestLine("writer", 9, 1.006, "F|42|cookie-edge|value"),
		traceMarkTestLine("writer", 9, 1.007, "F|42|cookie-edge|value "),
		traceMarkTestLine("writer", 9, 1.008, "S|42|"+longPrefix+"A|7"),
		traceMarkTestLine("writer", 9, 1.009, "S|42|"+longPrefix+"B|7"),
		traceMarkTestLine("writer", 9, 1.010, "F|42|"+longPrefix+"A|7"),
		traceMarkTestLine("writer", 9, 1.011, "F|42|"+longPrefix+"B|7"),
		traceMarkTestLine("writer", 9, 1.012, "G|42|track|work-a|7"),
		traceMarkTestLine("writer", 9, 1.013, "G|42|track |work-b|7"),
		traceMarkTestLine("writer", 9, 1.014, "H|42|track|7"),
		traceMarkTestLine("writer", 9, 1.015, "H|42|track |7"),
		// Mismatch-only controls: physical order must never substitute for the
		// complete typed name/track identity.
		traceMarkTestLine("writer", 9, 1.016, "S|42|mismatch|9"),
		traceMarkTestLine("writer", 9, 1.017, "F|42|mismatch |9"),
		traceMarkTestLine("writer", 9, 1.018, "G|42|only|work|8"),
		traceMarkTestLine("writer", 9, 1.019, "H|42|only |8"),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := Query{TimeStart: .999, TimeEnd: 1.020, TimeStartSet: true, TimeEndSet: true}
	stats := ComputeWindowStats(idx, q)
	if got := len(stats.TraceSpans); got != 6 {
		t.Fatalf("right-edge identities collapsed into a duplicate/mismatched S/F cohort: got=%d spans=%+v caveats=%v", got, stats.TraceSpans, stats.Caveats)
	}
	gotAsync := make(map[int]string, len(stats.TraceSpans))
	for _, span := range stats.TraceSpans {
		if span.Kind == "async" {
			gotAsync[span.StartLine] = span.Name
		}
	}
	wantAsync := map[int]string{
		1: "name", 2: "name ", 5: "cookie-edge", 6: "cookie-edge",
		9: longPrefix + "A", 10: longPrefix + "B",
	}
	if !reflect.DeepEqual(gotAsync, wantAsync) {
		t.Fatalf("full typed async identities drifted:\n got=%v\nwant=%v", gotAsync, wantAsync)
	}
	if got := len(stats.TraceTrackSpans); got != 2 {
		t.Fatalf("track and track-space collapsed into one G/H lane: got=%d spans=%+v caveats=%v", got, stats.TraceTrackSpans, stats.Caveats)
	}
	tracks := []string{stats.TraceTrackSpans[0].TrackName, stats.TraceTrackSpans[1].TrackName}
	sort.Strings(tracks)
	if !reflect.DeepEqual(tracks, []string{"track", "track "}) {
		t.Fatalf("track right-edge identity was normalized: %q", tracks)
	}
	for _, ev := range idx.Events {
		if ev.Line >= 9 && ev.Line <= 12 && len(ev.FieldText) != 300 {
			t.Fatalf("test premise failed: long marker inventory copy len=%d event=%+v", len(ev.FieldText), ev)
		}
	}
}

func TestTraceMarkBuildIndexAndStreamScanTypedParity(t *testing.T) {
	longTrack := strings.Repeat("t", 320) + " "
	longName := strings.Repeat("n", 320) + " "
	path := writeTraceMarkIntegrityTrace(t, "right-edge-stream-parity.systrace",
		traceMarkTestLine("writer", 9, 2.000, "B|42|phase "),
		traceMarkTestLine("writer", 9, 2.001, "E|42"),
		traceMarkTestLine("writer", 9, 2.002, "S|42|"+longName+"|cookie "),
		traceMarkTestLine("writer", 9, 2.003, "F|42|"+longName+"|cookie "),
		traceMarkTestLine("writer", 9, 2.004, "G|42|"+longTrack+"|work |7"),
		traceMarkTestLine("writer", 9, 2.005, "H|42|"+longTrack+"|7"),
		traceMarkTestLine("writer", 9, 2.006, "N|42|track |point "),
		traceMarkTestLine("writer", 9, 2.007, "I|42|instant "),
	)
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var streamed []Event
	streamIdx, err := StreamScan(context.Background(), path, TraceFlavorAuto, func(ev Event) bool {
		streamed = append(streamed, ev)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != len(streamed) {
		t.Fatalf("indexed/stream event count mismatch: %d vs %d", len(idx.Events), len(streamed))
	}
	for i := range idx.Events {
		indexed, stream := idx.Events[i], streamed[i]
		indexedTrack, streamTrack := traceTrackNameFromEvent(indexed), traceTrackNameFromEvent(stream)
		if indexed.Line != stream.Line || indexed.Type != stream.Type || indexed.FieldText != stream.FieldText || indexed.SpanAction != stream.SpanAction || indexed.SpanPID != stream.SpanPID || indexed.SpanName != stream.SpanName || indexed.SpanValue != stream.SpanValue || indexedTrack != streamTrack {
			t.Fatalf("indexed/stream typed marker mismatch at %d:\nindexed=%+v track=%q\n stream=%+v track=%q", i, indexed, indexedTrack, stream, streamTrack)
		}
	}
	if idx.Events[2].SpanName != longName || len(idx.Events[2].FieldText) != 300 {
		t.Fatalf("S full typed name did not survive FieldText clamp: %+v", idx.Events[2])
	}
	if got := traceTrackNameFromEvent(idx.Events[4]); got != longTrack || len(idx.Events[4].FieldText) != 300 {
		t.Fatalf("G full typed track did not survive FieldText clamp: track=%q event=%+v", got, idx.Events[4])
	}
	streamIdx.Events = streamed
	streamIdx.TimestampOrder = TraceTimestampOrderMonotonic
	query := Query{TimeStart: 1.999, TimeEnd: 2.010, TimeStartSet: true, TimeEndSet: true}
	for lane, candidate := range map[string]*Index{"indexed": idx, "stream": streamIdx} {
		stats := ComputeWindowStats(candidate, query)
		spanNames := map[string]string{}
		for _, span := range stats.TraceSpans {
			spanNames[span.Kind] = span.Name
		}
		if len(stats.TraceSpans) != 2 || spanNames["sync"] != "phase " || spanNames["async"] != longName {
			t.Fatalf("%s consumers lost B/S opaque names: spans=%+v caveats=%v", lane, stats.TraceSpans, stats.Caveats)
		}
		if len(stats.TraceTrackSpans) != 1 || stats.TraceTrackSpans[0].TrackName != longTrack || stats.TraceTrackSpans[0].Name != "work " {
			t.Fatalf("%s consumer reparsed the 300-byte G/H inventory copy: spans=%+v caveats=%v", lane, stats.TraceTrackSpans, stats.Caveats)
		}
		instantByAction := map[string]TraceInstantSummary{}
		for _, instant := range stats.TraceInstants {
			instantByAction[instant.Action] = instant
		}
		if len(stats.TraceInstants) != 2 || instantByAction["N"].TrackName != "track " || instantByAction["N"].Name != "point " || instantByAction["I"].Name != "instant " {
			t.Fatalf("%s instant consumers normalized N/I opaque edges: instants=%+v caveats=%v", lane, stats.TraceInstants, stats.Caveats)
		}
	}
	if ParserVersion != "tracequery-v29" {
		t.Fatalf("right-edge side-table schema changed without cache invalidation: %q", ParserVersion)
	}
}
