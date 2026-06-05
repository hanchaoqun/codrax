package llm

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// testAdapterOpts builds an AdapterOptions with valid sizing defaults
// for tests that only care about one specific knob. Mirrors what the
// factory layer would resolve when no yaml override is present.
func testAdapterOpts(opts AdapterOptions) AdapterOptions {
	if opts.RequestTimeout == 0 {
		opts.RequestTimeout = 120 * time.Second
	}
	if opts.RetryMaxAttempts == 0 {
		opts.RetryMaxAttempts = 6
	}
	return opts
}

// TestOpenAIAdapter_MaxContextTokens_Resolution pins the zero-
// sentinel fallback contract: a zero contextWindow (legacy
// providers.yaml without the field, or agent that forgot to
// override) returns DefaultContextWindow (200000 as of 2026-05-02)
// so any consumer assuming a positive return value stays safe. A
// declared positive value flows through unchanged.
func TestOpenAIAdapter_MaxContextTokens_Resolution(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero falls back to default 200K", 0, DefaultContextWindow},
		{"explicit small value wins", 8000, 8000},
		{"explicit large value wins", 1_000_000, 1_000_000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := NewOpenAIAdapter("k", "m", "http://x", testAdapterOpts(AdapterOptions{ContextWindow: c.in}))
			if got := a.MaxContextTokens(); got != c.want {
				t.Errorf("MaxContextTokens()=%d, want %d", got, c.want)
			}
		})
	}
}

// TestOpenAIAdapter_MaxOutputTokens_WireOmitWhenZero pins the load-
// bearing default for output capping: when MaxOutputTokens is zero,
// the wire payload MUST NOT include a max_tokens field, so the
// server falls back to the model's own output ceiling (matching
// every other LLM client). A positive value flows through unchanged.
func TestOpenAIAdapter_MaxOutputTokens_WireOmitWhenZero(t *testing.T) {
	msgs := []Message{{Role: "user", Content: "hi"}}

	t.Run("zero_max_output_omits_field", func(t *testing.T) {
		a := NewOpenAIAdapter("k", "m", "http://x", testAdapterOpts(AdapterOptions{MaxOutputTokens: 0}))
		b, _ := json.Marshal(a.buildRequest(msgs, nil, ChatOptions{}))
		if strings.Contains(string(b), `"max_tokens"`) {
			t.Errorf("zero MaxOutputTokens must omit max_tokens on the wire, got: %s", b)
		}
	})

	t.Run("positive_max_output_emits_field", func(t *testing.T) {
		a := NewOpenAIAdapter("k", "m", "http://x", testAdapterOpts(AdapterOptions{MaxOutputTokens: 8192}))
		b, _ := json.Marshal(a.buildRequest(msgs, nil, ChatOptions{}))
		if !strings.Contains(string(b), `"max_tokens":8192`) {
			t.Errorf("explicit MaxOutputTokens must serialize, got: %s", b)
		}
	})
}

func TestOpenAIAdapter_MultimodalContentPartsOnlyWhenRequested(t *testing.T) {
	adapter := &OpenAIAdapter{model: "m"}
	textReq := adapter.buildRequest([]Message{{Role: "user", Content: "hi"}}, nil, ChatOptions{})
	textRaw, err := json.Marshal(textReq.Messages[0].Content)
	if err != nil {
		t.Fatal(err)
	}
	if string(textRaw) != `"hi"` {
		t.Fatalf("text-only content = %s, want string", textRaw)
	}

	mmReq := adapter.buildRequest([]Message{{
		Role:    "user",
		Content: "inspect",
		ContentParts: []ContentPart{
			{Type: "image_url", ImageURL: "data:image/png;base64,abc", Detail: "high"},
		},
	}}, nil, ChatOptions{})
	raw, err := json.Marshal(mmReq.Messages[0].Content)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{`"type":"text"`, `"text":"inspect"`, `"type":"image_url"`, `"url":"data:image/png;base64,abc"`, `"detail":"high"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("multimodal content missing %s: %s", want, got)
		}
	}
}

// TestNewOpenAIAdapter_PanicsOnZeroSizing pins the fail-loud guards
// so a future caller cannot silently construct an adapter with a
// hung HTTP client (zero timeout) or a degenerate retry loop (zero
// attempts). The factory must apply the code default explicitly.
func TestNewOpenAIAdapter_PanicsOnZeroSizing(t *testing.T) {
	mustPanic := func(t *testing.T, opts AdapterOptions) {
		t.Helper()
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic, got none for opts=%+v", opts)
			}
		}()
		NewOpenAIAdapter("k", "m", "http://x", opts)
	}

	t.Run("zero_request_timeout_panics", func(t *testing.T) {
		mustPanic(t, AdapterOptions{RetryMaxAttempts: 6})
	})
	t.Run("zero_retry_attempts_panics", func(t *testing.T) {
		mustPanic(t, AdapterOptions{RequestTimeout: 120 * time.Second})
	})
	t.Run("zero_max_output_does_NOT_panic", func(t *testing.T) {
		// MaxOutputTokens = 0 is the recommended default ("don't
		// cap, server uses model ceiling") — must not panic.
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("zero MaxOutputTokens panicked: %v", r)
			}
		}()
		NewOpenAIAdapter("k", "m", "http://x", testAdapterOpts(AdapterOptions{MaxOutputTokens: 0}))
	})
}

// TestBuildRequest_ToolChoiceWire locks the wire-format contract for
// ChatOptions.ToolChoice. The OpenAI API takes `tool_choice` as either
// a string ("auto" / "required" / "none") or an object (force a
// specific function); codrax only forwards the string form today.
// Three invariants matter:
//   - empty or "auto" → field omitted (same payload as pre-options callers)
//   - "required" → field present as JSON string "required"
//   - tool_choice is never emitted when tools[] is empty (OpenAI rejects that)
func TestBuildRequest_ToolChoiceWire(t *testing.T) {
	schema := []ToolSchema{
		{Name: "emit_analysis", Description: "x", Parameters: json.RawMessage(`{"type":"object"}`)},
	}
	msgs := []Message{{Role: "user", Content: "hi"}}
	adapter := &OpenAIAdapter{model: "m"}

	t.Run("empty_options_omits_tool_choice", func(t *testing.T) {
		req := adapter.buildRequest(msgs, schema, ChatOptions{})
		b, _ := json.Marshal(req)
		if strings.Contains(string(b), `"tool_choice"`) {
			t.Errorf("empty options should omit tool_choice, got: %s", b)
		}
	})

	t.Run("auto_omits_tool_choice", func(t *testing.T) {
		req := adapter.buildRequest(msgs, schema, ChatOptions{ToolChoice: "auto"})
		b, _ := json.Marshal(req)
		if strings.Contains(string(b), `"tool_choice"`) {
			t.Errorf("auto should omit tool_choice (matches provider default), got: %s", b)
		}
	})

	t.Run("required_emits_string", func(t *testing.T) {
		req := adapter.buildRequest(msgs, schema, ChatOptions{ToolChoice: "required"})
		b, _ := json.Marshal(req)
		if !strings.Contains(string(b), `"tool_choice":"required"`) {
			t.Errorf("required should serialize as \"tool_choice\":\"required\", got: %s", b)
		}
	})

	t.Run("required_without_tools_still_omitted", func(t *testing.T) {
		req := adapter.buildRequest(msgs, nil, ChatOptions{ToolChoice: "required"})
		b, _ := json.Marshal(req)
		// Without tools[] an OpenAI 400 would rejects tool_choice; we
		// omit it so the adapter still produces a valid request when a
		// caller declared no tools but asked for required.
		if strings.Contains(string(b), `"tool_choice"`) {
			t.Errorf("empty tools should suppress tool_choice even when required, got: %s", b)
		}
	})
}

func TestBuildRequest_ProviderThinkingMode(t *testing.T) {
	msgs := []Message{{Role: "user", Content: "hi"}}
	schema := []ToolSchema{{Name: "emit_analysis", Parameters: json.RawMessage(`{"type":"object"}`)}}

	t.Run("deepseek auto disables native thinking", func(t *testing.T) {
		adapter := NewOpenAIAdapter("k", "deepseek-v4-flash", "https://api.deepseek.com", testAdapterOpts(AdapterOptions{}))
		req := adapter.buildRequest(msgs, schema, ChatOptions{ToolChoice: "required"})
		b, _ := json.Marshal(req)
		if !strings.Contains(string(b), `"thinking":{"type":"disabled"}`) {
			t.Fatalf("DeepSeek auto mode must actively disable provider thinking; got %s", b)
		}
		if !strings.Contains(string(b), `"tool_choice":"required"`) {
			t.Fatalf("disabling provider thinking must not suppress ordinary tool_choice; got %s", b)
		}
	})

	t.Run("non deepseek auto omits provider private field", func(t *testing.T) {
		adapter := NewOpenAIAdapter("k", "gpt-x", "https://example.test/v1", testAdapterOpts(AdapterOptions{}))
		req := adapter.buildRequest(msgs, schema, ChatOptions{})
		b, _ := json.Marshal(req)
		if strings.Contains(string(b), `"thinking"`) {
			t.Fatalf("unknown OpenAI-compatible providers must not receive DeepSeek-private thinking field by default; got %s", b)
		}
	})

	t.Run("explicit provider default omits even for deepseek", func(t *testing.T) {
		adapter := NewOpenAIAdapter("k", "deepseek-v4-flash", "https://api.deepseek.com/beta", testAdapterOpts(AdapterOptions{
			ProviderThinkingMode: "provider_default",
		}))
		req := adapter.buildRequest(msgs, schema, ChatOptions{})
		b, _ := json.Marshal(req)
		if strings.Contains(string(b), `"thinking"`) {
			t.Fatalf("provider_default must omit provider-native thinking fields; got %s", b)
		}
	})

	t.Run("explicit enabled is opt in", func(t *testing.T) {
		adapter := NewOpenAIAdapter("k", "deepseek-v4-pro", "https://api.deepseek.com", testAdapterOpts(AdapterOptions{
			ProviderThinkingMode: "enabled",
		}))
		req := adapter.buildRequest(msgs, schema, ChatOptions{})
		b, _ := json.Marshal(req)
		if !strings.Contains(string(b), `"thinking":{"type":"enabled"}`) {
			t.Fatalf("explicit enabled must be preserved for operators who opt in; got %s", b)
		}
	})

	t.Run("deepseek native thinking suppresses tool choice but keeps tools", func(t *testing.T) {
		adapter := NewOpenAIAdapter("k", "deepseek-v4-pro", "https://api.deepseek.com", testAdapterOpts(AdapterOptions{
			ProviderThinkingMode: "enabled",
		}))
		req := adapter.buildRequest(msgs, schema, ChatOptions{ToolChoice: "required"})
		b, _ := json.Marshal(req)
		if !strings.Contains(string(b), `"tools"`) {
			t.Fatalf("DeepSeek native thinking mode must still send tools; got %s", b)
		}
		if strings.Contains(string(b), `"tool_choice"`) {
			t.Fatalf("DeepSeek native thinking mode rejects tool_choice; adapter must omit it, got %s", b)
		}
	})

	t.Run("reasoning content round trips only when present", func(t *testing.T) {
		adapter := NewOpenAIAdapter("k", "deepseek-v4-pro", "https://api.deepseek.com", testAdapterOpts(AdapterOptions{}))
		req := adapter.buildRequest([]Message{{
			Role:             "assistant",
			Content:          "",
			ReasoningContent: "I need a tool.",
			ToolCalls: []ToolCall{{
				ID:     "call_1",
				Name:   "emit_analysis",
				Params: json.RawMessage(`{"question_kind":"mechanism"}`),
			}},
		}}, schema, ChatOptions{})
		b, _ := json.Marshal(req)
		if !strings.Contains(string(b), `"reasoning_content":"I need a tool."`) {
			t.Fatalf("assistant reasoning_content must round-trip when supplied; got %s", b)
		}
	})
}

func TestOpenAIAdapter_ReasoningContentParse(t *testing.T) {
	t.Run("non-stream response preserves reasoning content", func(t *testing.T) {
		body := []byte(`{"choices":[{"message":{"role":"assistant","reasoning_content":"plan","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"grep","arguments":"{\"pattern\":\"x\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":2,"completion_tokens":3}}`)
		adapter := &OpenAIAdapter{model: "m"}
		resp, err := adapter.parseResponse(body)
		if err != nil {
			t.Fatalf("parseResponse: %v", err)
		}
		if resp.ReasoningContent != "plan" {
			t.Fatalf("ReasoningContent = %q, want plan", resp.ReasoningContent)
		}
		if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "grep" {
			t.Fatalf("tool call not parsed: %+v", resp.ToolCalls)
		}
	})

	t.Run("stream response accumulates reasoning content without content delta", func(t *testing.T) {
		sse := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think \"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"more\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"emit_analysis\",\"arguments\":\"{}\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"
		var deltas []string
		var reasoningDeltas []string
		resp, err := parseSSEStream(strings.NewReader(sse),
			func(d string) { deltas = append(deltas, d) },
			func(d string) { reasoningDeltas = append(reasoningDeltas, d) },
			nil, nil, nil)
		if err != nil {
			t.Fatalf("parseSSEStream: %v", err)
		}
		if resp.ReasoningContent != "think more" {
			t.Fatalf("ReasoningContent = %q, want think more", resp.ReasoningContent)
		}
		if len(deltas) != 0 {
			t.Fatalf("reasoning_content must not be surfaced through content delta, got %+v", deltas)
		}
		if strings.Join(reasoningDeltas, "") != "think more" {
			t.Fatalf("reasoning deltas = %+v, want think more", reasoningDeltas)
		}
		if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "emit_analysis" {
			t.Fatalf("tool call not parsed: %+v", resp.ToolCalls)
		}
	})
}

func TestOpenAIAdapter_TextToolCallRecoveryOptIn(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, err := json.Marshal(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": "Here is the function call:\n\n```\n{\"name\":\"grep\",\"arguments\":{\"pattern\":\"Agent\",\"files_only\":true}}\n```",
				},
			}},
		})
		if err != nil {
			t.Fatalf("marshal test response: %v", err)
		}
		_, _ = w.Write(body)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	tools := []ToolSchema{{Name: "grep"}}
	opts := ChatOptions{ToolChoice: "required"}

	disabled := NewOpenAIAdapter("k", "m", server.URL, testAdapterOpts(AdapterOptions{}))
	resp, err := disabled.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, tools, opts)
	if err != nil {
		t.Fatalf("disabled Chat: %v", err)
	}
	if len(resp.ToolCalls) != 0 || resp.Content == "" {
		t.Fatalf("disabled recovery should leave assistant content untouched, got content=%q calls=%+v", resp.Content, resp.ToolCalls)
	}

	enabled := NewOpenAIAdapter("k", "m", server.URL, testAdapterOpts(AdapterOptions{RecoverTextToolCalls: true}))
	resp, err = enabled.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, tools, opts)
	if err != nil {
		t.Fatalf("enabled Chat: %v", err)
	}
	if resp.StopReason != "tool_use" || resp.Content != "" || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "grep" {
		t.Fatalf("enabled recovery should convert content envelope to tool call, got stop=%q content=%q calls=%+v",
			resp.StopReason, resp.Content, resp.ToolCalls)
	}
}

func TestOpenAIAdapter_StrictTextToolCallRecoveryAlwaysOn(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, err := json.Marshal(map[string]any{
			"choices": []any{map[string]any{
				"finish_reason": "stop",
				"message": map[string]any{
					"role":    "assistant",
					"content": `{"name":"grep","arguments":{"pattern":"Agent","files_only":true}}`,
				},
			}},
		})
		if err != nil {
			t.Fatalf("marshal test response: %v", err)
		}
		_, _ = w.Write(body)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	adapter := NewOpenAIAdapter("k", "m", server.URL, testAdapterOpts(AdapterOptions{}))
	resp, err := adapter.Chat(context.Background(), []Message{{Role: "user", Content: "x"}}, []ToolSchema{{
		Name:       "grep",
		Parameters: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string"},"files_only":{"type":"boolean"}},"required":["pattern"]}`),
	}}, ChatOptions{ToolChoice: "auto"})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.StopReason != "tool_use" || resp.Content != "" || len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "grep" {
		t.Fatalf("strict recovery should be always on for explicit envelopes, got stop=%q content=%q calls=%+v",
			resp.StopReason, resp.Content, resp.ToolCalls)
	}
}

// TestBuildHTTPClient_TLSOptions pins the http.Client construction
// path for per-provider TLS config:
//   - zero options → stock client, no custom Transport
//   - tls_ca_file set → client uses a Transport with RootCAs populated
//     and InsecureSkipVerify still false
//   - tls_insecure_skip_verify=true → Transport with InsecureSkipVerify=true
//   - unreadable tls_ca_file → error, no silent trust-pool fallback
func TestBuildHTTPClient_TLSOptions(t *testing.T) {
	t.Run("zero_options_uses_stock_client", func(t *testing.T) {
		c, err := buildHTTPClient(TLSOptions{}, "https://api.example.com/v1", 120*time.Second)
		if err != nil {
			t.Fatalf("buildHTTPClient: %v", err)
		}
		if c.Transport != nil {
			t.Errorf("zero options should leave Transport nil (use DefaultTransport), got %T", c.Transport)
		}
	})

	t.Run("insecure_skip_verify_sets_flag", func(t *testing.T) {
		c, err := buildHTTPClient(TLSOptions{InsecureSkipVerify: true}, "https://api.example.com/v1", 120*time.Second)
		if err != nil {
			t.Fatalf("buildHTTPClient: %v", err)
		}
		tr, ok := c.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("Transport type = %T, want *http.Transport", c.Transport)
		}
		if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
			t.Errorf("InsecureSkipVerify not propagated, got %+v", tr.TLSClientConfig)
		}
	})

	t.Run("ca_file_populates_root_cas", func(t *testing.T) {
		// Write a minimal valid PEM (self-generated test root).
		// This cert is ONLY used for the RootCAs load path; the test
		// never actually dials anything.
		pem := generateTestCAPEM(t)
		dir := t.TempDir()
		caPath := filepath.Join(dir, "ca.pem")
		if err := os.WriteFile(caPath, pem, 0o600); err != nil {
			t.Fatalf("write ca.pem: %v", err)
		}
		c, err := buildHTTPClient(TLSOptions{CAFile: caPath}, "https://api.example.com/v1", 120*time.Second)
		if err != nil {
			t.Fatalf("buildHTTPClient: %v", err)
		}
		tr, ok := c.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("Transport type = %T, want *http.Transport", c.Transport)
		}
		if tr.TLSClientConfig == nil || tr.TLSClientConfig.RootCAs == nil {
			t.Fatalf("RootCAs not populated: %+v", tr.TLSClientConfig)
		}
		if tr.TLSClientConfig.InsecureSkipVerify {
			t.Errorf("CAFile path must not flip InsecureSkipVerify on")
		}
	})

	t.Run("bad_ca_file_path_fails_loud", func(t *testing.T) {
		_, err := buildHTTPClient(TLSOptions{CAFile: "/does/not/exist/ca.pem"}, "https://api.example.com/v1", 120*time.Second)
		if err == nil {
			t.Fatal("missing tls_ca_file should return an error")
		}
		if msg := err.Error(); !strings.Contains(msg, "tls_ca_file") || !strings.Contains(msg, "cannot be read") {
			t.Fatalf("bad tls_ca_file error not actionable: %v", err)
		}
	})

	t.Run("zero_tls_cfg_does_not_leak_skip_verify", func(t *testing.T) {
		// Belt-and-suspenders: a CAFile-only client must not accidentally
		// enable InsecureSkipVerify on the Transport's fresh tls.Config.
		cfg := &tls.Config{}
		if cfg.InsecureSkipVerify {
			t.Errorf("fresh tls.Config should zero-init InsecureSkipVerify to false")
		}
	})
}

func TestNewFromConfigRejectsUnreadableTLSCAFile(t *testing.T) {
	missingCA := filepath.Join(t.TempDir(), "missing-ca.pem")
	_, err := NewFromConfig(types.LLMProviderConfig{
		Provider:  "openai",
		APIKey:    "k",
		Model:     "m",
		BaseURL:   "https://api.example.com/v1",
		TLSCAFile: missingCA,
	})
	if err == nil {
		t.Fatal("NewFromConfig should reject unreadable tls_ca_file")
	}
	if msg := err.Error(); !strings.Contains(msg, "tls_ca_file") || !strings.Contains(msg, missingCA) || !strings.Contains(msg, "cannot be read") {
		t.Fatalf("tls_ca_file error not actionable: %v", err)
	}
}

// TestParseSSEStream covers the streaming accumulator: content deltas
// append in order, tool-call arguments accumulate across chunks keyed
// by index, finish_reason lands in StopReason, and the final Response
// is shape-identical to what a non-streaming call would return.
func TestParseSSEStream(t *testing.T) {
	t.Run("content-only stream", func(t *testing.T) {
		sse := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello \"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"world\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
			"data: [DONE]\n"
		var deltas []string
		resp, err := parseSSEStream(strings.NewReader(sse), func(d string) { deltas = append(deltas, d) }, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("parseSSEStream: %v", err)
		}
		if resp.Content != "Hello world" {
			t.Errorf("Content = %q, want %q", resp.Content, "Hello world")
		}
		if resp.StopReason != "end_turn" {
			t.Errorf("StopReason = %q, want end_turn", resp.StopReason)
		}
		if len(resp.ToolCalls) != 0 {
			t.Errorf("ToolCalls = %v, want none", resp.ToolCalls)
		}
		if got := strings.Join(deltas, "|"); got != "Hello |world" {
			t.Errorf("onDelta ordering = %q, want %q", got, "Hello |world")
		}
	})

	t.Run("single tool call across chunks", func(t *testing.T) {
		sse := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_a\",\"function\":{\"name\":\"emit_analysis\",\"arguments\":\"\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"k\\\"\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\":42}\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n" +
			"data: [DONE]\n"
		resp, err := parseSSEStream(strings.NewReader(sse), nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("parseSSEStream: %v", err)
		}
		if resp.StopReason != "tool_use" {
			t.Errorf("StopReason = %q, want tool_use", resp.StopReason)
		}
		if len(resp.ToolCalls) != 1 {
			t.Fatalf("ToolCalls len = %d, want 1", len(resp.ToolCalls))
		}
		tc := resp.ToolCalls[0]
		if tc.ID != "call_a" || tc.Name != "emit_analysis" {
			t.Errorf("tool call meta = %+v, want id=call_a name=emit_analysis", tc)
		}
		if string(tc.Params) != `{"k":42}` {
			t.Errorf("tool call arguments = %q, want %q", string(tc.Params), `{"k":42}`)
		}
	})

	t.Run("multi tool calls interleaved by index", func(t *testing.T) {
		sse := "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"A\",\"function\":{\"name\":\"grep\",\"arguments\":\"\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":1,\"id\":\"B\",\"function\":{\"name\":\"read_file\",\"arguments\":\"\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\"{\\\"p\\\":1}\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":1,\"function\":{\"arguments\":\"{\\\"f\\\":\\\"a.go\\\"}\"}}]}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n" +
			"data: [DONE]\n"
		resp, err := parseSSEStream(strings.NewReader(sse), nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("parseSSEStream: %v", err)
		}
		if len(resp.ToolCalls) != 2 {
			t.Fatalf("ToolCalls len = %d, want 2", len(resp.ToolCalls))
		}
		if resp.ToolCalls[0].Name != "grep" || resp.ToolCalls[1].Name != "read_file" {
			t.Errorf("order = [%s, %s], want [grep, read_file]", resp.ToolCalls[0].Name, resp.ToolCalls[1].Name)
		}
		if string(resp.ToolCalls[0].Params) != `{"p":1}` {
			t.Errorf("grep params = %q", string(resp.ToolCalls[0].Params))
		}
		if string(resp.ToolCalls[1].Params) != `{"f":"a.go"}` {
			t.Errorf("read_file params = %q", string(resp.ToolCalls[1].Params))
		}
	})

	t.Run("empty stream errors", func(t *testing.T) {
		_, err := parseSSEStream(strings.NewReader(""), nil, nil, nil, nil, nil)
		if err == nil || !strings.Contains(err.Error(), "empty stream") {
			t.Errorf("expected empty-stream error, got %v", err)
		}
	})

	t.Run("malformed chunk skipped without crash", func(t *testing.T) {
		sse := "data: not-json\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
			"data: [DONE]\n"
		resp, err := parseSSEStream(strings.NewReader(sse), nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("parseSSEStream: %v", err)
		}
		if resp.Content != "ok" {
			t.Errorf("Content = %q, want ok", resp.Content)
		}
	})

	t.Run("usage chunk captured", func(t *testing.T) {
		sse := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":12,\"completion_tokens\":3}}\n\n" +
			"data: [DONE]\n"
		resp, err := parseSSEStream(strings.NewReader(sse), nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("parseSSEStream: %v", err)
		}
		if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 3 {
			t.Errorf("usage = %+v, want 12/3", resp.Usage)
		}
	})
}

// TestParseResponse_AutoDetectsSSE covers the fallback path for
// providers that return SSE even when `stream: false` was requested.
// parseResponse should recognise the `data:` prefix and route to the
// same accumulator used by the streaming path, so the caller gets a
// normal Response without knowing the provider was misbehaving.
func TestParseResponse_AutoDetectsSSE(t *testing.T) {
	a := &OpenAIAdapter{model: "m"}

	t.Run("real_sse_body_routed_to_stream_parser", func(t *testing.T) {
		body := []byte(
			"data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
				"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
				"data: [DONE]\n")
		resp, err := a.parseResponse(body)
		if err != nil {
			t.Fatalf("parseResponse: %v", err)
		}
		if resp.Content != "hi" {
			t.Errorf("Content = %q, want hi", resp.Content)
		}
	})

	t.Run("normal_json_body_still_works", func(t *testing.T) {
		body := []byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
		resp, err := a.parseResponse(body)
		if err != nil {
			t.Fatalf("parseResponse: %v", err)
		}
		if resp.Content != "ok" {
			t.Errorf("Content = %q, want ok", resp.Content)
		}
	})

	t.Run("leading_newlines_before_sse", func(t *testing.T) {
		// Blank lines before the first `data:` are legal SSE preamble
		// (some servers send heartbeats). looksLikeSSEResponse has to
		// trim leading whitespace; parseSSEStream handles blank lines
		// natively by ignoring any line that isn't prefixed with
		// `data:` at column 0.
		body := []byte("\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"x\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n")
		resp, err := a.parseResponse(body)
		if err != nil {
			t.Fatalf("parseResponse: %v", err)
		}
		if resp.Content != "x" {
			t.Errorf("Content = %q, want x", resp.Content)
		}
	})
}

// TestBuildRequest_StreamWireFormat locks the stream flag on the wire:
// streaming adapters set it to true, non-streaming omit it entirely
// so the payload is byte-identical to the pre-streaming implementation.
func TestBuildRequest_StreamWireFormat(t *testing.T) {
	msgs := []Message{{Role: "user", Content: "hi"}}

	t.Run("non_streaming_omits_field", func(t *testing.T) {
		a := &OpenAIAdapter{model: "m", stream: false}
		req := a.buildRequest(msgs, nil, ChatOptions{})
		b, _ := json.Marshal(req)
		if strings.Contains(string(b), `"stream"`) {
			t.Errorf("non-streaming payload must omit stream field, got: %s", b)
		}
	})

	t.Run("streaming_sets_true", func(t *testing.T) {
		a := &OpenAIAdapter{model: "m", stream: true}
		req := a.buildRequest(msgs, nil, ChatOptions{})
		b, _ := json.Marshal(req)
		if !strings.Contains(string(b), `"stream":true`) {
			t.Errorf("streaming payload must contain \"stream\":true, got: %s", b)
		}
	})
}

// generateTestCAPEM produces a valid self-signed certificate in PEM
// form. The cert is only used for the pool.AppendCertsFromPEM parse
// path; no TLS handshake actually touches it.
func generateTestCAPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "codrax-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// TestParseSSEStream_ProgressTrackerUpdated pins the contract that
// the SSE scanner updates the watchdog progress tracker on every
// scanned line. Without this, the doStreamRequest watchdog would
// false-positive on a slow-but-progressing stream and abort
// mid-emission.
//
// Bug provenance: eval Batch I+J sgf-parsing — planner LLM stream
// stalled mid-output and the http.Client.Timeout (120s) fired
// instead of an earlier stall-specific abort. Watchdog needs the
// tracker to distinguish "stream still progressing slowly" from
// "stream silently dead."
func TestParseSSEStream_ProgressTrackerUpdated(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n"
	var tracker atomic.Int64
	startNano := time.Now().UnixNano()
	tracker.Store(0) // start at zero so any update is visible
	_, err := parseSSEStream(strings.NewReader(sse), nil, nil, nil, &tracker, nil)
	if err != nil {
		t.Fatalf("parseSSEStream: %v", err)
	}
	got := tracker.Load()
	if got == 0 {
		t.Fatal("progress tracker was never updated by SSE scanner — watchdog would falsely trip")
	}
	if got < startNano {
		t.Errorf("tracker timestamp %d earlier than test start %d — clock anomaly?", got, startNano)
	}
}

// TestParseSSEStream_NilTrackerStillWorks ensures the
// progress-tracker arg is nil-safe so parseSSEStream remains usable
// from contexts without stall detection (e.g. parseResponse's
// SSE-auto-detect path on a non-streaming endpoint).
func TestParseSSEStream_NilTrackerStillWorks(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n"
	resp, err := parseSSEStream(strings.NewReader(sse), nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("nil tracker should not fail: %v", err)
	}
	if resp.Content != "hi" {
		t.Errorf("Content = %q, want hi", resp.Content)
	}
}

// TestStreamStallTimeout_SaneDefaults pins the constants that the
// watchdog uses. Bumping these silently could either re-introduce
// the 120s burn (if too long) or false-positive on slow models (if
// too short).
func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 4, 26, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		in   string
		want time.Duration
	}{
		{"empty", "", 0},
		{"unparseable", "garbage", 0},
		{"integer_seconds", "30", 30 * time.Second},
		{"zero_seconds", "0", 0},
		{"large_capped_to_1h", "9999", time.Hour},
		{"http_date_future", "Sun, 26 Apr 2026 12:01:00 GMT", time.Minute},
		{"http_date_past_returns_zero", "Sun, 26 Apr 2026 11:59:00 GMT", 0},
		{"with_whitespace", "  10  ", 10 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseRetryAfter(tc.in, now)
			if got != tc.want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestLooksLikeQuotaError(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"anthropic_rate_limit", `{"error":{"type":"rate_limit_error","message":"x"}}`, true},
		{"openai_insufficient_quota", `{"error":{"code":"insufficient_quota"}}`, true},
		{"openai_rate_limit_exceeded", `{"error":{"code":"rate_limit_exceeded"}}`, true},
		{"vendor_token_plan", `{"error":{"message":"Token Plan limit reached"}}`, true},
		{"chinese_quota_message", `{"error":{"message":"当前请求量较高，请稍后重试"}}`, true},
		{"chinese_quota_marker", `{"error":{"message":"配额已耗尽"}}`, true},
		{"deployment_rotation", `{"error":{"message":"No deployments available for selected model"}}`, false},
		{"empty", ``, false},
		{"non_quota_error", `{"error":{"message":"internal server error"}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := looksLikeQuotaError([]byte(tc.body))
			if got != tc.want {
				t.Errorf("looksLikeQuotaError(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

func TestNextRetryDelay_Precedence(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		attempt int
		want    time.Duration
	}{
		{
			name:    "retry_after_takes_precedence",
			err:     &apiError{StatusCode: 429, RetryAfter: 45 * time.Second, QuotaError: true},
			attempt: 0,
			want:    45 * time.Second,
		},
		{
			name:    "quota_error_uses_long_schedule_attempt0",
			err:     &apiError{StatusCode: 429, QuotaError: true},
			attempt: 0,
			want:    8 * time.Second,
		},
		{
			name:    "quota_error_uses_long_schedule_attempt2",
			err:     &apiError{StatusCode: 429, QuotaError: true},
			attempt: 2,
			want:    32 * time.Second,
		},
		{
			name:    "deployment_rotation_uses_short_schedule_attempt0",
			err:     &apiError{StatusCode: 429, QuotaError: false},
			attempt: 0,
			want:    2 * time.Second,
		},
		{
			name:    "5xx_uses_short_schedule_attempt2",
			err:     &apiError{StatusCode: 503},
			attempt: 2,
			want:    8 * time.Second,
		},
		{
			name:    "non_apierror_falls_through_to_default",
			err:     fmt.Errorf("some random error"),
			attempt: 0,
			want:    2 * time.Second,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nextRetryDelay(tc.err, tc.attempt)
			if got != tc.want {
				t.Errorf("nextRetryDelay(%v, attempt=%d) = %v, want %v", tc.err, tc.attempt, got, tc.want)
			}
		})
	}
}

func TestNewAPIError_ParsesHeadersAndBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: 429,
		Header:     http.Header{"Retry-After": []string{"15"}},
	}
	body := []byte(`{"error":{"type":"rate_limit_error","message":"plan limit"}}`)
	ae := newAPIError(resp, body)
	if ae.StatusCode != 429 {
		t.Errorf("StatusCode = %d, want 429", ae.StatusCode)
	}
	if ae.RetryAfter != 15*time.Second {
		t.Errorf("RetryAfter = %v, want 15s", ae.RetryAfter)
	}
	if !ae.QuotaError {
		t.Error("QuotaError should be true for rate_limit_error body")
	}

	// 5xx with no Retry-After and a generic body — neither header nor
	// quota classification should fire.
	resp2 := &http.Response{StatusCode: 502, Header: http.Header{}}
	ae2 := newAPIError(resp2, []byte(`{"error":"upstream timeout"}`))
	if ae2.RetryAfter != 0 {
		t.Errorf("RetryAfter = %v, want 0", ae2.RetryAfter)
	}
	if ae2.QuotaError {
		t.Error("QuotaError should be false for non-429 status")
	}
}

// TestStreamHTTPClient_HasNoOuterTimeout pins the load-bearing
// invariant: the streaming client must NOT impose an outer
// http.Client.Timeout. Streaming body cancellation is owned by the
// per-request context + streamStallTimeout watchdog. The transport
// still needs a ResponseHeaderTimeout so a provider that never
// starts the stream cannot hang before the watchdog exists.
func TestStreamHTTPClient_HasNoOuterTimeout(t *testing.T) {
	a := NewOpenAIAdapter("k", "m", "http://x", testAdapterOpts(AdapterOptions{
		RequestTimeout:   120 * time.Second,
		RetryMaxAttempts: 6,
	}))
	if a.streamHTTPClient == nil {
		t.Fatal("streamHTTPClient must be constructed alongside httpClient")
	}
	if a.streamHTTPClient.Timeout != 0 {
		t.Errorf("streamHTTPClient.Timeout = %v, want 0 (watchdog owns cancellation)", a.streamHTTPClient.Timeout)
	}
	tr, ok := a.streamHTTPClient.Transport.(*http.Transport)
	if !ok || tr == nil {
		t.Fatalf("streamHTTPClient.Transport = %T, want *http.Transport", a.streamHTTPClient.Transport)
	}
	if tr.ResponseHeaderTimeout != defaultStreamFirstByteTimeout {
		t.Errorf("stream ResponseHeaderTimeout = %v, want first-byte timeout %v", tr.ResponseHeaderTimeout, defaultStreamFirstByteTimeout)
	}
	if a.httpClient.Timeout != 120*time.Second {
		t.Errorf("httpClient.Timeout = %v, want 120s (non-stream cap unchanged)", a.httpClient.Timeout)
	}
}

func TestStreamStallTimeout_SaneDefaults(t *testing.T) {
	if defaultStreamStallTimeout < 10*time.Second {
		t.Errorf("defaultStreamStallTimeout=%v is too aggressive; thinking-model inter-chunk pauses can hit 5-10s, would false-positive", defaultStreamStallTimeout)
	}
	// Upper bound widened to 120s in 2026-05-18: deep-thinking models
	// (DeepSeek-R1 / o1-style) routinely pause 60s+ between
	// thinking blocks. The previous 60s ceiling fired the watchdog
	// mid-stream on legitimate slow upstreams. Outer http timeout is
	// owned by RequestTimeout (default 240s), and the stream client
	// has NO outer timeout, so the upper bound is a sanity guard
	// rather than a coupling to the http path.
	if defaultStreamStallTimeout > 120*time.Second {
		t.Errorf("defaultStreamStallTimeout=%v is too lax; should be ≤120s to keep the upper bound disciplined", defaultStreamStallTimeout)
	}
	if streamStallTickInterval >= defaultStreamStallTimeout {
		t.Errorf("tick interval %v must be < stall timeout %v so detection has resolution", streamStallTickInterval, defaultStreamStallTimeout)
	}
}

func TestStreamFirstByteTimeout_SaneDefaults(t *testing.T) {
	if defaultStreamFirstByteTimeout < 40*time.Second {
		t.Errorf("defaultStreamFirstByteTimeout=%v is too aggressive; slow upstream models can need 30-40s before the first usable SSE event", defaultStreamFirstByteTimeout)
	}
	if defaultStreamFirstByteTimeout > defaultStreamStallTimeout {
		t.Errorf("defaultStreamFirstByteTimeout=%v must not exceed stall timeout %v", defaultStreamFirstByteTimeout, defaultStreamStallTimeout)
	}
	if streamStallTickInterval >= defaultStreamFirstByteTimeout {
		t.Errorf("tick interval %v must be < first-byte timeout %v so detection has resolution", streamStallTickInterval, defaultStreamFirstByteTimeout)
	}
}
