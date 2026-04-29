package llm

import (
	"strings"
	"sync/atomic"
	"testing"
)

// TestParseSSEStream_ToolCallDelta_FiresPerChunk locks the contract
// that onToolCallDelta receives EACH delta of the tool-call args
// with index + name + chunk. Useful for live-preview consumers that
// want to see tool args streaming in.
func TestParseSSEStream_ToolCallDelta_FiresPerChunk(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"emit_answer_document","arguments":"{\"sum"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"mary\":\"hello"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":" world\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		"data: [DONE]",
	}, "\n\n") + "\n\n"

	type capture struct {
		index int
		name  string
		chunk string
	}
	var captured []capture
	cb := func(idx int, name, chunk string) {
		captured = append(captured, capture{idx, name, chunk})
	}
	resp, err := parseSSEStream(strings.NewReader(sse), nil, cb, nil, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(captured) != 3 {
		t.Fatalf("expected 3 chunks, got %d: %+v", len(captured), captured)
	}
	wantChunks := []string{`{"sum`, `mary":"hello`, ` world"}`}
	for i, c := range captured {
		if c.index != 0 {
			t.Errorf("chunk[%d] index = %d, want 0", i, c.index)
		}
		if c.chunk != wantChunks[i] {
			t.Errorf("chunk[%d] = %q, want %q", i, c.chunk, wantChunks[i])
		}
	}
	// The first chunk arrives before the function name has been seen
	// in the SSE delta (some providers send name on first chunk; this
	// test's first chunk DOES include the name). After first chunk
	// the name is sticky.
	if captured[0].name != "emit_answer_document" {
		t.Errorf("chunk[0] name = %q, want emit_answer_document", captured[0].name)
	}
	if captured[len(captured)-1].name != "emit_answer_document" {
		t.Errorf("last chunk name = %q, want emit_answer_document", captured[len(captured)-1].name)
	}
	// I2 — full args buffer is byte-identical to expected JSON
	// regardless of callback presence.
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	got := string(resp.ToolCalls[0].Params)
	want := `{"summary":"hello world"}`
	if got != want {
		t.Errorf("accumulated args = %q, want %q", got, want)
	}
}

// TestParseSSEStream_ToolCallDelta_NilCallbackHasNoEffect proves
// that passing nil for onToolCallDelta is byte-identical to the
// pre-2026-04-29 behaviour. Required because every existing test
// passes nil and must keep working.
func TestParseSSEStream_ToolCallDelta_NilCallbackHasNoEffect(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"x","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}` + "\n\n" +
		"data: [DONE]\n\n"
	resp, err := parseSSEStream(strings.NewReader(sse), nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse with nil callbacks: %v", err)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.ToolCalls))
	}
	if string(resp.ToolCalls[0].Params) != "{}" {
		t.Errorf("args = %q, want '{}'", string(resp.ToolCalls[0].Params))
	}
}

// TestParseSSEStream_ToolCallDelta_ByteIdenticalArgs is the I2
// invariant: tool-call accumulator output is byte-identical with
// vs without the callback installed. This is the load-bearing
// guarantee that the live-preview path (which uses the callback)
// cannot disturb the canonical AnswerDocument parse downstream.
func TestParseSSEStream_ToolCallDelta_ByteIdenticalArgs(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"emit","arguments":"{\"a\":1"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":",\"b\":\"x\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		"data: [DONE]",
	}, "\n\n") + "\n\n"

	respA, err := parseSSEStream(strings.NewReader(sse), nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("parse without cb: %v", err)
	}
	cb := func(int, string, string) {} // observe but do nothing
	respB, err := parseSSEStream(strings.NewReader(sse), nil, cb, nil, nil)
	if err != nil {
		t.Fatalf("parse with cb: %v", err)
	}
	if string(respA.ToolCalls[0].Params) != string(respB.ToolCalls[0].Params) {
		t.Errorf("byte-identity violated: without=%q with=%q",
			respA.ToolCalls[0].Params, respB.ToolCalls[0].Params)
	}
}

// TestParseSSEStream_ToolCallDelta_ProgressUnaffected: the watchdog
// progress tracker fires on every line scan regardless of which
// callbacks are installed. Locks the invariant that the new hook
// does not skip / dedupe progress updates.
func TestParseSSEStream_ToolCallDelta_ProgressUnaffected(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"x"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"y"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		"data: [DONE]",
	}, "\n\n") + "\n\n"
	var progress atomic.Int64
	cb := func(int, string, string) {}
	_, err := parseSSEStream(strings.NewReader(sse), nil, cb, &progress, nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if progress.Load() == 0 {
		t.Error("progress tracker should have fired at least once")
	}
}
