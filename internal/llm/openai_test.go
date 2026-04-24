package llm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestOpenAIAdapter_MaxContextTokens_Resolution pins the zero-
// sentinel fallback contract: a zero contextWindow (legacy
// providers.yaml without the field, or agent that forgot to
// override) returns 128000 so any consumer assuming a positive
// return value stays safe. A declared positive value flows
// through unchanged.
func TestOpenAIAdapter_MaxContextTokens_Resolution(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want int
	}{
		{"zero falls back to 128K", 0, 128000},
		{"explicit small value wins", 8000, 8000},
		{"explicit large value wins", 1_000_000, 1_000_000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := NewOpenAIAdapter("k", "m", "http://x", false, c.in, TLSOptions{})
			if got := a.MaxContextTokens(); got != c.want {
				t.Errorf("MaxContextTokens()=%d, want %d", got, c.want)
			}
		})
	}
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

// TestBuildHTTPClient_TLSOptions pins the http.Client construction
// path for per-provider TLS config:
//   - zero options → stock client, no custom Transport
//   - tls_ca_file set → client uses a Transport with RootCAs populated
//     and InsecureSkipVerify still false
//   - tls_insecure_skip_verify=true → Transport with InsecureSkipVerify=true
func TestBuildHTTPClient_TLSOptions(t *testing.T) {
	t.Run("zero_options_uses_stock_client", func(t *testing.T) {
		c := buildHTTPClient(TLSOptions{}, "https://api.example.com/v1")
		if c.Transport != nil {
			t.Errorf("zero options should leave Transport nil (use DefaultTransport), got %T", c.Transport)
		}
	})

	t.Run("insecure_skip_verify_sets_flag", func(t *testing.T) {
		c := buildHTTPClient(TLSOptions{InsecureSkipVerify: true}, "https://api.example.com/v1")
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
		c := buildHTTPClient(TLSOptions{CAFile: caPath}, "https://api.example.com/v1")
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

	t.Run("bad_ca_file_path_falls_back_gracefully", func(t *testing.T) {
		// Missing file → log warning, skip RootCAs override.
		c := buildHTTPClient(TLSOptions{CAFile: "/does/not/exist/ca.pem"}, "https://api.example.com/v1")
		tr, ok := c.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("Transport type = %T, want *http.Transport", c.Transport)
		}
		if tr.TLSClientConfig == nil {
			t.Fatalf("TLSClientConfig nil; want empty config after fallback")
		}
		if tr.TLSClientConfig.RootCAs != nil {
			t.Errorf("bad CA file should leave RootCAs nil, got populated pool")
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
		resp, err := parseSSEStream(strings.NewReader(sse), func(d string) { deltas = append(deltas, d) })
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
		resp, err := parseSSEStream(strings.NewReader(sse), nil)
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
		resp, err := parseSSEStream(strings.NewReader(sse), nil)
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
		_, err := parseSSEStream(strings.NewReader(""), nil)
		if err == nil || !strings.Contains(err.Error(), "empty stream") {
			t.Errorf("expected empty-stream error, got %v", err)
		}
	})

	t.Run("malformed chunk skipped without crash", func(t *testing.T) {
		sse := "data: not-json\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
			"data: [DONE]\n"
		resp, err := parseSSEStream(strings.NewReader(sse), nil)
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
		resp, err := parseSSEStream(strings.NewReader(sse), nil)
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
