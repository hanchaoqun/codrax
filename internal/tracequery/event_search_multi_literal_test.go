package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEventSearchPatternsAreLiteralORAndPatternPipeStaysLiteral(t *testing.T) {
	idx := &Index{Events: []Event{
		{Type: EventTraceMark, Line: 1, Ts: 1.000, SpanName: "VerifyClass Demo"},
		{Type: EventTraceMark, Line: 2, Ts: 1.001, SpanName: "JIT compile Demo"},
		{Type: EventTraceMark, Line: 3, Ts: 1.002, SpanName: "VerifyClass|JIT literal"},
	}}

	orRows := EventSearch(idx, Query{Patterns: []string{"VerifyClass", "JIT"}, Limit: 10})
	if len(orRows) != 3 {
		t.Fatalf("patterns OR rows=%d, want 3: %+v", len(orRows), orRows)
	}
	literalPipe := EventSearch(idx, Query{Pattern: "VerifyClass|JIT", Limit: 10})
	if len(literalPipe) != 1 || literalPipe[0].Line != 3 {
		t.Fatalf("legacy pattern must retain literal pipe semantics: %+v", literalPipe)
	}
}

func TestStreamEventSearchPatternsMatchesIndexedLiteralOR(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.systrace")
	trace := "app-20 (20) [001] .... 1.000000: tracing_mark_write: B|20|VerifyClass Demo\n" +
		"app-20 (20) [001] .... 1.001000: tracing_mark_write: E|20\n" +
		"jit-30 (30) [002] .... 1.002000: tracing_mark_write: B|30|JIT compile Demo\n"
	if err := os.WriteFile(path, []byte(trace), 0o644); err != nil {
		t.Fatal(err)
	}
	q := Query{
		View:       "event_search",
		Patterns:   []string{"VerifyClass", "JIT"},
		EventTypes: []EventType{EventTraceMark},
		Limit:      10,
	}
	streamed, err := StreamEventSearch(context.Background(), path, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(streamed.Events) != 2 {
		t.Fatalf("stream rows=%d, want 2: %+v", len(streamed.Events), streamed.Events)
	}
	idx, err := BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	indexed := EventSearch(idx, q)
	if len(indexed) != len(streamed.Events) {
		t.Fatalf("indexed/stream parity mismatch: indexed=%d streamed=%d", len(indexed), len(streamed.Events))
	}
}
