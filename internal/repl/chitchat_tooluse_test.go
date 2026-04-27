package repl

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// scriptedChatAdapter is a single-method llm.Adapter test double
// that replays a queue of pre-canned responses. Each call to Chat
// pops one response off the head; the test asserts both the call
// count and the inputs the responder forwarded.
type scriptedChatAdapter struct {
	mu        sync.Mutex
	responses []llm.Response
	calls     []scriptedCall
}

type scriptedCall struct {
	messages []llm.Message
	tools    []llm.ToolSchema
}

func (a *scriptedChatAdapter) Chat(messages []llm.Message, tools []llm.ToolSchema, opts llm.ChatOptions) (llm.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, scriptedCall{
		messages: append([]llm.Message(nil), messages...),
		tools:    append([]llm.ToolSchema(nil), tools...),
	})
	if len(a.responses) == 0 {
		return llm.Response{}, errors.New("scriptedChatAdapter: response queue empty")
	}
	resp := a.responses[0]
	a.responses = a.responses[1:]
	// Mimic streaming when the test wired a callback so the round-2
	// streamed-content branch is also exercised.
	if opts.OnContentDelta != nil && resp.Content != "" {
		opts.OnContentDelta(resp.Content)
	}
	return resp, nil
}

// ModelID and other Adapter methods are stubs — the tool-use loop
// only calls Chat. Defining the rest keeps llm.Adapter satisfied.
func (a *scriptedChatAdapter) ModelID() string                  { return "scripted" }
func (a *scriptedChatAdapter) MaxContextTokens() int            { return 4096 }
func (a *scriptedChatAdapter) MaxOutputTokens() int             { return 0 }
func (a *scriptedChatAdapter) RequestTimeout() time.Duration   { return 0 }
func (a *scriptedChatAdapter) RetryMaxAttempts() int            { return 0 }
func (a *scriptedChatAdapter) ModelMetadata() map[string]string { return nil }

// stubMemReader is a minimal types.MemoryReader. The tool-use loop
// hands it the LLM-produced query verbatim; the test asserts the
// query reaches Search and that the canned IndexEntry slice is
// rendered into the second-round Chat input.
type stubMemReader struct {
	called  bool
	lastQ   string
	lastOpt types.MemorySearchOpts
	results []types.MemoryIndexEntry
}

func (s *stubMemReader) Search(q string, opts types.MemorySearchOpts) []types.MemoryIndexEntry {
	s.called = true
	s.lastQ = q
	s.lastOpt = opts
	return s.results
}

// TestChitchatToolUse_NoToolCallShortCircuits verifies the round-1
// no-tool-call path: when the LLM answers directly without a
// tool_use block, RespondWithMemory returns the content immediately
// and never touches MemoryReader. Round 2 is also skipped, so the
// scripted adapter receives exactly one Chat call.
func TestChitchatToolUse_NoToolCallShortCircuits(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			{Content: "你好!", StopReason: "end_turn"},
		},
	}
	mem := &stubMemReader{}
	r := &llmChitchatResponder{adapter: adapter}
	reply, err := r.RespondWithMemory("你好", "", mem, nil)
	if err != nil {
		t.Fatalf("RespondWithMemory: %v", err)
	}
	if reply != "你好!" {
		t.Errorf("reply = %q, want '你好!'", reply)
	}
	if mem.called {
		t.Errorf("MemoryReader.Search called for non-tool-use turn (greeting); should be skipped")
	}
	if len(adapter.calls) != 1 {
		t.Errorf("Chat called %d times, want 1 (no second round when no tool_use)", len(adapter.calls))
	}
	// Round 1 must offer the recall_memory schema so the LLM CAN
	// invoke it when relevant. Drift guard: schema must be wired.
	if len(adapter.calls[0].tools) == 0 || adapter.calls[0].tools[0].Name != "recall_memory" {
		t.Errorf("round 1 did not advertise recall_memory tool; got tools=%+v", adapter.calls[0].tools)
	}
}

// TestChitchatToolUse_ToolCallDispatchesAndAnswers covers the
// happy-path tool-use flow: LLM emits a tool_use → MemoryReader.Search
// runs with the LLM's query → second-round Chat receives the tool
// message and returns the final reply. Locks the 2-Chat-call shape
// + the memory-reader call + the round-2 messages including the
// tool result.
func TestChitchatToolUse_ToolCallDispatchesAndAnswers(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			{
				Content: "let me check",
				ToolCalls: []llm.ToolCall{{
					ID: "call-1", Name: "recall_memory",
					Params: json.RawMessage(`{"query": "OAuth refactor", "limit": 5}`),
				}},
				StopReason: "tool_use",
			},
			{Content: "我们之前讨论过 OAuth 重构 (转 JWT)", StopReason: "end_turn"},
		},
	}
	mem := &stubMemReader{
		results: []types.MemoryIndexEntry{{
			ID: "turn-1", Topic: "oauth migration",
			Summary: "moved tokens to JWT", Keywords: []string{"oauth", "jwt"},
			Kind: "plan",
		}},
	}
	r := &llmChitchatResponder{adapter: adapter}
	reply, err := r.RespondWithMemory("我们之前聊过 OAuth 吗?", "", mem, nil)
	if err != nil {
		t.Fatalf("RespondWithMemory: %v", err)
	}
	if reply != "我们之前讨论过 OAuth 重构 (转 JWT)" {
		t.Errorf("reply = %q", reply)
	}
	if !mem.called {
		t.Fatal("MemoryReader.Search should have been called")
	}
	if mem.lastQ != "OAuth refactor" {
		t.Errorf("Search query = %q; want LLM's interpreted query 'OAuth refactor'", mem.lastQ)
	}
	if mem.lastOpt.Limit != 5 {
		t.Errorf("Search limit = %d; want 5 from LLM args", mem.lastOpt.Limit)
	}
	if len(adapter.calls) != 2 {
		t.Fatalf("Chat called %d times, want 2 (round 1 with tool, round 2 to answer)", len(adapter.calls))
	}
	// Round 2 must include the tool message with the rendered
	// recall result so the LLM can synthesise the final reply.
	r2 := adapter.calls[1]
	foundTool := false
	for _, m := range r2.messages {
		if m.Role == "tool" && m.ToolCallID == "call-1" {
			foundTool = true
			if !strings.Contains(m.Content, "oauth migration") {
				t.Errorf("round 2 tool message missing recall content; got %q", m.Content)
			}
		}
	}
	if !foundTool {
		t.Errorf("round 2 messages missing tool reply; got %+v", r2.messages)
	}
	if len(r2.tools) != 0 {
		t.Errorf("round 2 must not advertise tools again (one-call cap); got tools=%v", r2.tools)
	}
}

// TestChitchatToolUse_NilMemoryFallsBackToOneShot verifies the
// memory-unwired fallback: with mem=nil, RespondWithMemory routes
// to the legacy single-call Respond/RespondStream path so existing
// behaviour is byte-identical. No tool offered.
func TestChitchatToolUse_NilMemoryFallsBackToOneShot(t *testing.T) {
	adapter := &scriptedChatAdapter{
		responses: []llm.Response{
			{Content: "你好!", StopReason: "end_turn"},
		},
	}
	r := &llmChitchatResponder{adapter: adapter}
	reply, err := r.RespondWithMemory("hi", "", nil, nil)
	if err != nil {
		t.Fatalf("RespondWithMemory: %v", err)
	}
	if reply != "你好!" {
		t.Errorf("reply = %q", reply)
	}
	// Fallback path must NOT advertise the recall tool (memory unwired).
	if len(adapter.calls[0].tools) != 0 {
		t.Errorf("nil-memory fallback should not offer tools; got %+v", adapter.calls[0].tools)
	}
}

// TestChitchatRecall_LimitClampedByConfig locks that yaml-supplied
// limits flow into the dispatcher: a tool_use asking for limit=99
// gets clamped to RecallMaxLimit before Search runs.
func TestChitchatRecall_LimitClampedByConfig(t *testing.T) {
	mem := &stubMemReader{}
	settings := types.ChitchatSettings{RecallDefaultLimit: 5, RecallMaxLimit: 7}
	dispatchChitchatRecall(llm.ToolCall{
		ID: "x", Name: "recall_memory",
		Params: json.RawMessage(`{"query": "X", "limit": 99}`),
	}, mem, settings)
	if mem.lastOpt.Limit != 7 {
		t.Errorf("oversize limit clamped to %d, want 7 (RecallMaxLimit)", mem.lastOpt.Limit)
	}
}

// TestChitchatRecall_MissingLimitUsesDefault locks the zero-limit
// path: tool_use without `limit` gets the configured default, not
// a stale package constant.
func TestChitchatRecall_MissingLimitUsesDefault(t *testing.T) {
	mem := &stubMemReader{}
	settings := types.ChitchatSettings{RecallDefaultLimit: 3, RecallMaxLimit: 12}
	dispatchChitchatRecall(llm.ToolCall{
		ID: "x", Name: "recall_memory",
		Params: json.RawMessage(`{"query": "X"}`),
	}, mem, settings)
	if mem.lastOpt.Limit != 3 {
		t.Errorf("missing limit should use RecallDefaultLimit=3; got %d", mem.lastOpt.Limit)
	}
}
