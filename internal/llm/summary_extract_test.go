package llm

import (
	"strings"
	"testing"
)

// TestSummaryExtractor_HappyPath: summary as the first field, simple
// ASCII string, single chunk. The decoded text equals the JSON
// content between the quotes; done flips at the closing quote.
func TestSummaryExtractor_HappyPath(t *testing.T) {
	var e SummaryExtractor
	out, done := e.Feed(`{"summary": "hello world", "shape": "explanation"}`)
	if !done {
		t.Fatalf("done = false, want true after closing quote")
	}
	if out != "hello world" {
		t.Errorf("decoded = %q, want %q", out, "hello world")
	}
}

// TestSummaryExtractor_ChunkedAcrossKey: chunks split right inside
// the `summary` key. Extractor must accumulate the key across
// chunks and recognise it.
func TestSummaryExtractor_ChunkedAcrossKey(t *testing.T) {
	var e SummaryExtractor
	chunks := []string{`{"sum`, `mary": "hello"`, `, "shape": "x"}`}
	var out strings.Builder
	for _, c := range chunks {
		s, _ := e.Feed(c)
		out.WriteString(s)
	}
	if out.String() != "hello" {
		t.Errorf("decoded = %q, want %q", out.String(), "hello")
	}
}

// TestSummaryExtractor_EscapeSequences: standard JSON escapes are
// decoded to their literal characters in the preview output.
func TestSummaryExtractor_EscapeSequences(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`{"summary":"line1\nline2"}`, "line1\nline2"},
		{`{"summary":"tab\there"}`, "tab\there"},
		{`{"summary":"quote\""}`, `quote"`},
		{`{"summary":"slash\\here"}`, `slash\here`},
		{`{"summary":"你好"}`, "你好"},
	}
	for _, tc := range cases {
		var e SummaryExtractor
		got, _ := e.Feed(tc.in)
		if got != tc.want {
			t.Errorf("decode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestSummaryExtractor_ChunkSplitInEscape: escape sequence split
// across chunks (e.g. "\\" and "n" arriving separately) must
// re-assemble correctly via the carry-over buffer.
func TestSummaryExtractor_ChunkSplitInEscape(t *testing.T) {
	var e SummaryExtractor
	parts := []string{`{"summary":"a\`, `nb"}`}
	var out strings.Builder
	for _, p := range parts {
		s, _ := e.Feed(p)
		out.WriteString(s)
	}
	if out.String() != "a\nb" {
		t.Errorf("decoded = %q, want %q", out.String(), "a\nb")
	}
}

// TestSummaryExtractor_ChunkSplitInUnicodeEscape: 你 split
// across chunks must wait for full hex before emitting.
func TestSummaryExtractor_ChunkSplitInUnicodeEscape(t *testing.T) {
	var e SummaryExtractor
	parts := []string{`{"summary":"\u4f`, `60"}`}
	var out strings.Builder
	for _, p := range parts {
		s, _ := e.Feed(p)
		out.WriteString(s)
	}
	if out.String() != "你" {
		t.Errorf("decoded = %q, want %q", out.String(), "你")
	}
}

// TestSummaryExtractor_NotFirstField: fields before summary are
// skipped, summary is still extracted.
func TestSummaryExtractor_NotFirstField(t *testing.T) {
	var e SummaryExtractor
	out, _ := e.Feed(`{"shape":"explanation","citations":[],"summary":"x"}`)
	if out != "x" {
		t.Errorf("decoded = %q, want %q", out, "x")
	}
}

// TestSummaryExtractor_StructuredFieldBefore: an object-valued field
// before summary (like citations array) must be skipped without
// confusing the state machine.
func TestSummaryExtractor_StructuredFieldBefore(t *testing.T) {
	var e SummaryExtractor
	out, _ := e.Feed(`{"citations":[{"file":"a.go","line":1}],"summary":"y"}`)
	if out != "y" {
		t.Errorf("decoded = %q, want %q", out, "y")
	}
}

// TestSummaryExtractor_DoneIsIdempotent: feeding chunks after done
// returns no further output.
func TestSummaryExtractor_DoneIsIdempotent(t *testing.T) {
	var e SummaryExtractor
	_, done := e.Feed(`{"summary":"x"}`)
	if !done {
		t.Fatal("expected done=true")
	}
	out, done2 := e.Feed(`,"more":"data"`)
	if out != "" || !done2 {
		t.Errorf("post-done Feed should be no-op; got out=%q done=%v", out, done2)
	}
}

// TestSummaryExtractor_SummaryAbsent: JSON with no summary key at
// all — extractor never returns text, never marks done while
// chunks have content. Caller's stream-end Clear handles cleanup.
func TestSummaryExtractor_SummaryAbsent(t *testing.T) {
	var e SummaryExtractor
	out, done := e.Feed(`{"shape":"value","value":{"literal":"42"}}`)
	if out != "" {
		t.Errorf("decoded = %q, want empty (no summary)", out)
	}
	if done {
		t.Error("done=true but stream had no closing summary boundary")
	}
}

// TestSummaryExtractor_EmptyInput: empty chunk is a no-op.
func TestSummaryExtractor_EmptyInput(t *testing.T) {
	var e SummaryExtractor
	out, done := e.Feed("")
	if out != "" || done {
		t.Errorf("empty feed should be no-op; got out=%q done=%v", out, done)
	}
}

// TestSummaryExtractor_NilSafe: methods on nil receiver behave
// gracefully.
func TestSummaryExtractor_NilSafe(t *testing.T) {
	var e *SummaryExtractor
	out, done := e.Feed("anything")
	if out != "" || !done {
		t.Errorf("nil receiver should return ('', true); got out=%q done=%v", out, done)
	}
}
