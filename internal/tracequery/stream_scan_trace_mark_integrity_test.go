package tracequery

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func streamTraceMarkIntegrityShell(t *testing.T, path string) (*Index, []Event) {
	t.Helper()
	var events []Event
	shell, err := StreamScan(context.Background(), path, TraceFlavorAuto, func(ev Event) bool {
		events = append(events, ev)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	return shell, events
}

func assertStreamTraceMarkIntegrityParity(t *testing.T, path string, shell *Index) *Index {
	t.Helper()
	indexed, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(shell.traceMarkIntegrityFailures, indexed.traceMarkIntegrityFailures) ||
		shell.traceMarkIntegrityFailuresCapped != indexed.traceMarkIntegrityFailuresCapped ||
		shell.traceMarkIntegrityDroppedGlobalPoison != indexed.traceMarkIntegrityDroppedGlobalPoison ||
		shell.traceTrackIntegrityDroppedPoison != indexed.traceTrackIntegrityDroppedPoison {
		t.Fatalf("stream/index trace-mark integrity drift:\nstream failures=%+v capped=%t global=%t track=%t\nindex  failures=%+v capped=%t global=%t track=%t",
			shell.traceMarkIntegrityFailures, shell.traceMarkIntegrityFailuresCapped,
			shell.traceMarkIntegrityDroppedGlobalPoison, shell.traceTrackIntegrityDroppedPoison,
			indexed.traceMarkIntegrityFailures, indexed.traceMarkIntegrityFailuresCapped,
			indexed.traceMarkIntegrityDroppedGlobalPoison, indexed.traceTrackIntegrityDroppedPoison)
	}
	if got, want := traceMarkIntegrityCaveats(shell, Query{}), traceMarkIntegrityCaveats(indexed, Query{}); !reflect.DeepEqual(got, want) {
		t.Fatalf("stream/index trace-mark caveats drift:\nstream=%v\nindex=%v", got, want)
	}
	return indexed
}

func TestStreamScanTraceMarkIntegrityMatchesBuildIndexForMaterializedAndUnmaterializedRows(t *testing.T) {
	unsafeTimestamp := "1" + strings.Repeat("0", 305) + ".0"
	path := writeTraceMarkIntegrityTrace(t, "stream-integrity.systrace",
		traceMarkTestLine("app", 10, 1.000, "B|bad|materialized"),
		` app-10 (10) [9999] .... 1.001000: tracing_mark_write: G|10|track|work|7`,
		" app-10 (10) [000] .... "+unsafeTimestamp+`: tracing_mark_write: E`,
		` app-999999999999999999999999 (10) [000] .... 1.003000: tracing_mark_write: H|10|track|7`,
		traceMarkTestLine("app", 10, 1.004, "N|10||point"),
		traceMarkTestLine("app", 10, 1.005, "I|10|"),
		`W Logger: quoted user text tracing_mark_write: B|999|not-a-row`,
	)
	shell, events := streamTraceMarkIntegrityShell(t, path)
	assertStreamTraceMarkIntegrityParity(t, path, shell)

	if len(shell.traceMarkIntegrityFailures) != 6 {
		t.Fatalf("stream shell lost malformed trace-mark rows: %+v", shell.traceMarkIntegrityFailures)
	}
	wantActions := []string{"B", "G", "E", "H", "N", "I"}
	wantReasons := []string{"invalid_payload_pid", "invalid_header_cpu", "invalid_timestamp", "invalid_emitter_pid", "empty_track_name", "empty_name"}
	wantUnmaterialized := []bool{false, true, true, true, false, false}
	for i, failure := range shell.traceMarkIntegrityFailures {
		if failure.Action != wantActions[i] || failure.Reason != wantReasons[i] {
			t.Errorf("failure %d = %+v, want action=%s reason=%s", i, failure, wantActions[i], wantReasons[i])
		}
		if failure.Line != i+1 || failure.LocalLine != i+1 || failure.SourcePath != canonicalTraceIndexPath(path) {
			t.Errorf("failure %d lost physical provenance: %+v", i, failure)
		}
		if failure.Unmaterialized != wantUnmaterialized[i] {
			t.Errorf("failure %d materialization state=%t, want %t: %+v", i, failure.Unmaterialized, wantUnmaterialized[i], failure)
		}
	}
	materializedMalformedLines := map[int]bool{}
	for _, ev := range events {
		if ev.Type == EventTraceMark && ev.SpanAction == "" {
			materializedMalformedLines[ev.Line] = true
		}
		if ev.Line >= 2 && ev.Line <= 4 {
			t.Errorf("unmaterialized trace-mark row leaked into callback as Event: %+v", ev)
		}
	}
	if want := map[int]bool{1: true, 5: true, 6: true}; !reflect.DeepEqual(materializedMalformedLines, want) {
		t.Fatalf("materialized malformed rows did not reach the event callback: got=%v want=%v events=%+v", materializedMalformedLines, want, events)
	}
	if shell.UnparsedLines == 0 {
		t.Fatal("fixture must retain ordinary non-ftrace prose as unparsed quality data")
	}
	for _, failure := range shell.traceMarkIntegrityFailures {
		if failure.Line == 7 {
			t.Fatalf("ordinary prose quoting trace_mark became a hard-gate witness: %+v", failure)
		}
	}
	if !traceMarkUnknownEmitterFailureForQuery(shell, Query{}) {
		t.Fatal("unmaterialized classic endpoint must reach the global fail-closed state")
	}
	if failure := traceTrackIntegrityFailureForQuery(shell, Query{}); failure == nil || failure.Action != "G" {
		t.Fatalf("unmaterialized G/H endpoint must reach track integrity state: %+v", failure)
	}
}

func TestStreamScanTraceMarkIntegrityQueryBoundaryAndUnparsedProseIsolation(t *testing.T) {
	path := writeTraceMarkIntegrityTrace(t, "stream-integrity-window.systrace",
		traceMarkTestLine("app", 10, .500, "B|bad|before"),
		traceMarkTestLine("app", 10, 1.500, "S|10||inside-cookie"),
		traceMarkTestLine("app", 10, 2.500, "F|10|after|"),
		`?corrupt? [003] tracing_mark_write: E`,
		`log: replayed text tracing_mark_write: G|10|track|quoted|1`,
	)
	shell, _ := streamTraceMarkIntegrityShell(t, path)
	assertStreamTraceMarkIntegrityParity(t, path, shell)

	q := Query{TimeStart: 1, TimeEnd: 2, TimeStartSet: true, TimeEndSet: true}
	var relevantLines []int
	for _, failure := range shell.traceMarkIntegrityFailures {
		if traceMarkIntegrityFailureRelevantToQuery(failure, q) {
			relevantLines = append(relevantLines, failure.Line)
		}
	}
	// A pre-window endpoint can corrupt carry-in state and remains relevant;
	// a known-timestamp row after the upper edge is excluded. An
	// unmaterialized row with no timestamp cannot be proven to be after the
	// window and therefore remains fail-closed.
	if want := []int{1, 2, 4}; !reflect.DeepEqual(relevantLines, want) {
		t.Fatalf("window relevance drifted: got lines=%v want=%v failures=%+v", relevantLines, want, shell.traceMarkIntegrityFailures)
	}
	if len(shell.traceMarkIntegrityFailures) != 4 {
		t.Fatalf("quoted prose must not add an integrity failure: %+v", shell.traceMarkIntegrityFailures)
	}

	proseOnly := writeTraceMarkIntegrityTrace(t, "stream-integrity-prose.systrace",
		traceMarkTestLine("app", 10, 3.000, "B|10|healthy"),
		`W Logger: quoted user text tracing_mark_write: E`,
		traceMarkTestLine("app", 10, 3.002, "E"),
	)
	proseShell, _ := streamTraceMarkIntegrityShell(t, proseOnly)
	assertStreamTraceMarkIntegrityParity(t, proseOnly, proseShell)
	if proseShell.UnparsedLines == 0 || len(proseShell.traceMarkIntegrityFailures) != 0 ||
		traceMarkUnknownEmitterFailureForQuery(proseShell, Query{}) || traceTrackIntegrityFailureForQuery(proseShell, Query{}) != nil {
		t.Fatalf("ordinary unparsed prose became a trace-mark hard gate: unparsed=%d failures=%+v global=%t track=%+v",
			proseShell.UnparsedLines, proseShell.traceMarkIntegrityFailures,
			traceMarkUnknownEmitterFailureForQuery(proseShell, Query{}), traceTrackIntegrityFailureForQuery(proseShell, Query{}))
	}
}

func TestStreamScanTraceMarkIntegrityCapAndDroppedPoisonMatchBuildIndex(t *testing.T) {
	t.Run("known materialized overflow stays local", func(t *testing.T) {
		var lines []string
		for i := 0; i < traceMarkIntegrityFailureCap+1; i++ {
			lines = append(lines, traceMarkTestLine("app", 10, 4+float64(i)/1000, "B|bad|known"))
		}
		path := writeTraceMarkIntegrityTrace(t, "stream-known-cap.systrace", lines...)
		shell, _ := streamTraceMarkIntegrityShell(t, path)
		assertStreamTraceMarkIntegrityParity(t, path, shell)
		if len(shell.traceMarkIntegrityFailures) != traceMarkIntegrityFailureCap ||
			!shell.traceMarkIntegrityFailuresCapped || shell.traceMarkIntegrityDroppedGlobalPoison || shell.traceTrackIntegrityDroppedPoison {
			t.Fatalf("known materialized cap semantics drifted: len=%d capped=%t global=%t track=%t",
				len(shell.traceMarkIntegrityFailures), shell.traceMarkIntegrityFailuresCapped,
				shell.traceMarkIntegrityDroppedGlobalPoison, shell.traceTrackIntegrityDroppedPoison)
		}
	})

	t.Run("dropped classic and track poison survive cap", func(t *testing.T) {
		var lines []string
		for i := 0; i < traceMarkIntegrityFailureCap; i++ {
			lines = append(lines, traceMarkTestLine("app", 10, 5+float64(i)/1000, "B|bad|known"))
		}
		lines = append(lines,
			` app-999999999999999999999999 (10) [000] .... 5.080000: tracing_mark_write: E`,
			traceMarkTestLine("app", 10, 5.081, "G|10||name|1"),
		)
		path := writeTraceMarkIntegrityTrace(t, "stream-dropped-cap.systrace", lines...)
		shell, _ := streamTraceMarkIntegrityShell(t, path)
		assertStreamTraceMarkIntegrityParity(t, path, shell)
		if len(shell.traceMarkIntegrityFailures) != traceMarkIntegrityFailureCap ||
			!shell.traceMarkIntegrityFailuresCapped || !shell.traceMarkIntegrityDroppedGlobalPoison || !shell.traceTrackIntegrityDroppedPoison {
			t.Fatalf("dropped poison cap semantics drifted: len=%d capped=%t global=%t track=%t",
				len(shell.traceMarkIntegrityFailures), shell.traceMarkIntegrityFailuresCapped,
				shell.traceMarkIntegrityDroppedGlobalPoison, shell.traceTrackIntegrityDroppedPoison)
		}
		if !caveatsContain(traceMarkIntegrityCaveats(shell, Query{}), "additional_rows_unknown=true") {
			t.Fatalf("ledger cap disclosure missing: %v", traceMarkIntegrityCaveats(shell, Query{}))
		}
	})
}
