package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/memory"
)

// summarizerStubAdapter lets each test control exactly what the LLM
// returns without any network or real provider. Fields cover the
// full surface the llmSummarizer inspects: ToolCalls[0].Name,
// ToolCalls[0].Params, plus optional chat-level error.
type summarizerStubAdapter struct {
	resp llm.Response
	err  error

	// Capture the last Chat invocation so tests can assert the
	// summarizer built the expected messages and asked for a tool
	// call rather than a free-form answer.
	lastMessages []llm.Message
	lastTools    []llm.ToolSchema
	lastOpts     llm.ChatOptions
}

func (s *summarizerStubAdapter) Chat(messages []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	s.lastMessages = messages
	s.lastTools = tools
	s.lastOpts = opts
	return s.resp, s.err
}

func (s *summarizerStubAdapter) ModelID() string                  { return "stub-v1" }
func (s *summarizerStubAdapter) MaxContextTokens() int            { return 100000 }
func (s *summarizerStubAdapter) MaxOutputTokens() int             { return 0 }
func (s *summarizerStubAdapter) RequestTimeout() time.Duration    { return 120 * time.Second }
func (s *summarizerStubAdapter) RetryMaxAttempts() int            { return 6 }

func newSummarizerTurn() memory.Turn {
	return memory.Turn{
		ID:       "turn-42",
		Request:  "how does the analyzer build the TermGraph?",
		Response: "The normalizer walks the request and produces canonical TermGraph nodes.",
	}
}

// TestLLMSummarizer_ValidToolCallProducesStructuredIndexEntry is the
// golden path: the stub adapter emits a well-formed tool_call and the
// summarizer unpacks it into an IndexEntry preserving all three
// fields verbatim.
func TestLLMSummarizer_ValidToolCallProducesStructuredIndexEntry(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{
		"topic":    "TermGraph construction",
		"keywords": []string{"normalizer", "TermGraph", "analyzer"},
		"summary":  "Normalizer walks the request and emits canonical TermGraph nodes the downstream pipeline consumes.",
	})
	adapter := &summarizerStubAdapter{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{
				{ID: "call-1", Name: memorySummarizerTool.Name, Params: params},
			},
		},
	}
	s := newLLMSummarizer(adapter)

	entry, err := s.Summarize(context.Background(), newSummarizerTurn())
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if entry.ID != "turn-42" {
		t.Errorf("ID: got %q, want %q", entry.ID, "turn-42")
	}
	if entry.Topic != "TermGraph construction" {
		t.Errorf("Topic: got %q, want structured value", entry.Topic)
	}
	if !reflect.DeepEqual(entry.Keywords, []string{"normalizer", "TermGraph", "analyzer"}) {
		t.Errorf("Keywords: got %v, want structured slice", entry.Keywords)
	}
	if entry.Summary == "" || entry.Summary == newSummarizerTurn().Response {
		t.Errorf("Summary: got %q, want LLM-emitted text", entry.Summary)
	}

	// Contract check: ToolChoice must be required so a compliant
	// provider cannot return prose instead of a tool call.
	if adapter.lastOpts.ToolChoice != "required" {
		t.Errorf("ToolChoice: got %q, want %q", adapter.lastOpts.ToolChoice, "required")
	}
	if len(adapter.lastTools) != 1 || adapter.lastTools[0].Name != memorySummarizerTool.Name {
		t.Errorf("tools schema: got %+v, want single emit_memory_summary", adapter.lastTools)
	}
}

// TestLLMSummarizer_NilAdapterReturnsFallback preserves the
// pre-session-30 behaviour: no provider configured → summarizer
// silently falls through to the keyword-extraction heuristic so
// memory compaction still produces a usable entry.
func TestLLMSummarizer_NilAdapterReturnsFallback(t *testing.T) {
	s := newLLMSummarizer(nil)
	entry, err := s.Summarize(context.Background(), newSummarizerTurn())
	if err != nil {
		t.Fatalf("nil adapter should not error: %v", err)
	}
	if entry.Topic == "" || entry.Summary == "" {
		t.Errorf("fallback must populate Topic/Summary; got %+v", entry)
	}
	if len(entry.Keywords) == 0 {
		t.Errorf("fallback must populate Keywords; got %v", entry.Keywords)
	}
}

// TestLLMSummarizer_ChatErrorFallsBack verifies the fail-safe: a
// transient provider error (network, 5xx, timeout) must not bubble
// up and break memory compaction — the summarizer returns a
// heuristic IndexEntry instead.
func TestLLMSummarizer_ChatErrorFallsBack(t *testing.T) {
	adapter := &summarizerStubAdapter{err: errors.New("upstream 503")}
	s := newLLMSummarizer(adapter)
	entry, err := s.Summarize(context.Background(), newSummarizerTurn())
	if err != nil {
		t.Fatalf("chat error must be swallowed into fallback, got err=%v", err)
	}
	if entry.ID != "turn-42" {
		t.Errorf("ID preserved on fallback; got %q", entry.ID)
	}
	if entry.Topic == "" {
		t.Errorf("fallback Topic must be populated; got empty")
	}
}

// TestLLMSummarizer_MissingToolCallFallsBack verifies the "LLM
// forgot to emit the tool" path. Previously this would have been a
// JSON parse error on an empty Content string, silently producing a
// keyword-heuristic entry via the old parse-error branch; the new
// contract routes it to the same fallback explicitly.
func TestLLMSummarizer_MissingToolCallFallsBack(t *testing.T) {
	adapter := &summarizerStubAdapter{
		resp: llm.Response{Content: "I'm sorry, I can't do that."},
	}
	s := newLLMSummarizer(adapter)
	entry, err := s.Summarize(context.Background(), newSummarizerTurn())
	if err != nil {
		t.Fatalf("missing tool_call must fall back, got err=%v", err)
	}
	// Fallback Topic is truncated request; cannot equal the LLM's
	// prose refusal.
	if entry.Topic == "I'm sorry, I can't do that." {
		t.Errorf("fallback must not leak refusal content into Topic")
	}
}

// TestLLMSummarizer_MalformedToolParamsFallBack verifies the last
// fail-safe: even when the LLM emits the tool call, if the arguments
// JSON is malformed the summarizer must not panic or return empty —
// the heuristic fallback covers the turn.
func TestLLMSummarizer_MalformedToolParamsFallBack(t *testing.T) {
	adapter := &summarizerStubAdapter{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{
				{Name: memorySummarizerTool.Name, Params: json.RawMessage(`{not valid json`)},
			},
		},
	}
	s := newLLMSummarizer(adapter)
	entry, err := s.Summarize(context.Background(), newSummarizerTurn())
	if err != nil {
		t.Fatalf("malformed params must fall back, got err=%v", err)
	}
	if entry.Topic == "" || entry.Summary == "" {
		t.Errorf("fallback must populate Topic+Summary; got %+v", entry)
	}
}

// TestLLMSummarizer_PartialFieldsBackfilledFromFallback verifies the
// partial-fill defence: if the LLM emits the tool with one field
// empty, the summarizer substitutes the heuristic for the missing
// field only, preserving the LLM's work on the filled fields.
func TestLLMSummarizer_PartialFieldsBackfilledFromFallback(t *testing.T) {
	params, _ := json.Marshal(map[string]interface{}{
		"topic":    "TermGraph construction",
		"keywords": []string{}, // LLM forgot keywords
		"summary":  "The normalizer walks the request.",
	})
	adapter := &summarizerStubAdapter{
		resp: llm.Response{
			ToolCalls: []llm.ToolCall{
				{Name: memorySummarizerTool.Name, Params: params},
			},
		},
	}
	s := newLLMSummarizer(adapter)
	entry, err := s.Summarize(context.Background(), newSummarizerTurn())
	if err != nil {
		t.Fatalf("partial fill must not error: %v", err)
	}
	if entry.Topic != "TermGraph construction" {
		t.Errorf("non-empty Topic from LLM must be preserved; got %q", entry.Topic)
	}
	if len(entry.Keywords) == 0 {
		t.Errorf("empty Keywords from LLM must be backfilled from heuristic; got empty")
	}
}
