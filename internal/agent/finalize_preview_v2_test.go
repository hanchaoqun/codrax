package agent

import (
	"strings"
	"testing"
)

// ── B3-T6 V2 partial salvage tests ───────────────────────────────

func TestV2PartialFromArgsChunk_ExtractsBlockSummaryText(t *testing.T) {
	raw := `{"document_model":"v2","blocks":[{"id":"b1","kind":"summary","text":"Hello world"}]}`
	got := v2PartialFromArgsChunk(raw)
	if got != "Hello world" {
		t.Errorf("got %q want %q", got, "Hello world")
	}
}

func TestV2PartialFromArgsChunk_HandlesEscapeSequences(t *testing.T) {
	raw := `{"document_model":"v2","blocks":[{"id":"b1","kind":"summary","text":"line1\nline2 with \"quote\""}]}`
	got := v2PartialFromArgsChunk(raw)
	want := "line1\nline2 with \"quote\""
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestV2PartialFromArgsChunk_TruncatedStreamReturnsPartialText(t *testing.T) {
	// Stream cut mid-text — helper should return whatever it has.
	raw := `{"document_model":"v2","blocks":[{"id":"b1","kind":"summary","text":"truncated mid-`
	got := v2PartialFromArgsChunk(raw)
	if !strings.Contains(got, "truncated mid-") {
		t.Errorf("expected partial text; got %q", got)
	}
}

func TestV2PartialFromArgsChunk_NoV1MarkerReturnsEmpty(t *testing.T) {
	raw := `{"shape":"explanation","summary":"V1 emit"}`
	if got := v2PartialFromArgsChunk(raw); got != "" {
		t.Errorf("V1-only stream must return empty; got %q", got)
	}
}

func TestV2PartialFromArgsChunk_NoSummaryBlockReturnsEmpty(t *testing.T) {
	// V2 emit with only ordered_list, no summary block.
	raw := `{"document_model":"v2","blocks":[{"id":"l1","kind":"ordered_list"}]}`
	if got := v2PartialFromArgsChunk(raw); got != "" {
		t.Errorf("expected empty; got %q", got)
	}
}

func TestV2PartialFromArgsChunk_RespectsFirstSummaryBlock(t *testing.T) {
	// Two summary blocks — helper extracts the first one.
	raw := `{"document_model":"v2","blocks":[
		{"id":"b1","kind":"summary","text":"first"},
		{"id":"b2","kind":"summary","text":"second"}
	]}`
	got := v2PartialFromArgsChunk(raw)
	if got != "first" {
		t.Errorf("expected first summary; got %q", got)
	}
}
